package cmd

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wolfigs/webnak-cli/internal/api"
	"github.com/wolfigs/webnak-cli/internal/ui"
)

// Hosting is a hosted site's state.
type Hosting struct {
	Subdomain     string   `json:"subdomain"`
	URL           string   `json:"url"`
	Source        string   `json:"source"`
	Repo          string   `json:"repo"`
	Branch        string   `json:"branch"`
	Template      string   `json:"template"`
	CurrentDeploy string   `json:"currentDeploy"`
	Deploys       []Deploy `json:"deploys"`
	DraftChanges  int      `json:"draftChanges"`
	CanBuild      bool     `json:"canBuild"`
	Build         any      `json:"build"`
}

// Deploy is one deploy of a hosted site.
type Deploy struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Phase     string    `json:"phase"`
	Error     string    `json:"error"`
	Commit    string    `json:"commit"`
	Files     int       `json:"files"`
	Bytes     int64     `json:"bytes"`
	CreatedAt time.Time `json:"createdAt"`
	Log       string    `json:"log"`
}

func (a *App) hosting(siteID string) (*Hosting, error) {
	var out struct {
		Hosting *Hosting `json:"hosting"`
	}
	if err := a.API.Do(a.Ctx(), "GET", "/api/v1/webnak/sites/"+api.Path(siteID)+"/hosting", nil, &out); err != nil {
		if api.IsStatus(err, 404) {
			return nil, fmt.Errorf("this site isn't hosted by Webnak (it's a site hosted elsewhere, made editable)")
		}
		return nil, err
	}
	return out.Hosting, nil
}

func latestDeploy(h *Hosting) *Deploy {
	if h == nil || len(h.Deploys) == 0 {
		return nil
	}
	d := h.Deploys
	sort.SliceStable(d, func(i, j int) bool { return d[i].CreatedAt.After(d[j].CreatedAt) })
	return &d[0]
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	return s
}

func init() {
	sitesCmd := &cobra.Command{
		Use:     "sites",
		Aliases: []string{"site", "ls"},
		Short:   "List your sites (and manage one)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.needAuth(); err != nil {
				return err
			}
			all, err := app.sites()
			if err != nil {
				return err
			}
			app.Out.Result(all, func() {
				if len(all) == 0 {
					app.Out.Print("No sites yet. Make one: %s", app.Out.Cyan("webnak create"))
					return
				}
				rows := [][]string{}
				for _, s := range all {
					rows = append(rows, []string{s.Name, s.Address(), s.Status(), s.ID})
				}
				app.Out.Table([]string{"NAME", "ADDRESS", "STATUS", "ID"}, rows)
			})
			return nil
		},
	}

	sitesCmd.AddCommand(&cobra.Command{
		Use:   "info <site>",
		Short: "Show a site: address, status, deploys",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.pickSite(argOr(args, 0))
			if err != nil {
				return err
			}
			var h *Hosting
			if s.Hosted != nil {
				h, _ = app.hosting(s.ID)
			}
			app.Out.Result(map[string]any{"site": s, "hosting": h}, func() {
				o := app.Out
				o.Print("%s  %s", o.Bold(s.Name), o.Dim(s.ID))
				o.Print("  Address:  %s", o.Cyan(s.Address()))
				o.Print("  Status:   %s", s.Status())
				if s.TakenDown {
					o.Print("  Reason:   %s", s.TakedownReason)
				}
				if h != nil {
					o.Print("  Source:   %s%s", h.Source, map[bool]string{true: " (" + h.Repo + ")", false: ""}[h.Repo != ""])
					if h.DraftChanges > 0 {
						o.Print("  Changes:  %d file change(s) not deployed yet (`webnak files publish %s`)", h.DraftChanges, s.Ref())
					}
					if d := latestDeploy(h); d != nil {
						o.Print("  Latest:   %s, %s, %d files", d.Status, d.CreatedAt.Local().Format("2 Jan 15:04"), d.Files)
					}
				}
			})
			return nil
		},
	})

	var delYes bool
	del := &cobra.Command{
		Use:   "delete <site>",
		Short: "Delete a site (its hosting, edits and history)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			if !app.Yes && !delYes {
				if !app.Out.Interactive() {
					return fmt.Errorf("deleting %s can't be undone; pass --yes to confirm", s.Name)
				}
				if !app.Out.Confirm(fmt.Sprintf("Delete %s (%s)? This can't be undone.", s.Name, s.Address()), false) {
					return fmt.Errorf("cancelled")
				}
			}
			if err := app.API.Do(app.Ctx(), "DELETE", "/api/v1/webnak/sites/"+api.Path(s.ID), nil, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true, "deleted": s.ID}, func() { app.Out.Success("Deleted %s.", s.Name) })
			return nil
		},
	}
	del.Flags().BoolVar(&delYes, "yes", false, "don't ask")
	sitesCmd.AddCommand(del)

	sitesCmd.AddCommand(&cobra.Command{
		Use:   "rename <site> <new name>",
		Short: "Rename a site",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			if err := app.API.Do(app.Ctx(), "PATCH", "/api/v1/webnak/sites/"+api.Path(s.ID), map[string]string{"name": args[1]}, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true}, func() { app.Out.Success("Renamed to %s.", args[1]) })
			return nil
		},
	})

	sitesCmd.AddCommand(&cobra.Command{
		Use:   "open <site>",
		Short: "Open a site in the browser (--dashboard for its dashboard page)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.pickSite(argOr(args, 0))
			if err != nil {
				return err
			}
			u := s.Address()
			if dash, _ := cmd.Flags().GetBool("dashboard"); dash || u == "" {
				u = DashboardURL() + "/webnak/sites/" + s.ID
			}
			if !ui.OpenBrowser(u) {
				app.Out.Print("%s", u)
			}
			app.Out.Result(map[string]any{"url": u}, nil)
			return nil
		},
	})
	sitesCmd.Commands()[len(sitesCmd.Commands())-1].Flags().Bool("dashboard", false, "open the dashboard page instead")
	root.AddCommand(sitesCmd)

	root.AddCommand(&cobra.Command{
		Use:     "templates",
		Aliases: []string{"template"},
		Short:   "List the templates you can start a site from",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.needAuth(); err != nil {
				return err
			}
			var out struct {
				Templates []struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					Category    string `json:"category"`
					Description string `json:"description"`
				} `json:"templates"`
			}
			if err := app.API.Do(app.Ctx(), "GET", "/api/v1/hosting/templates", nil, &out); err != nil {
				return err
			}
			app.Out.Result(out.Templates, func() {
				rows := [][]string{}
				for _, t := range out.Templates {
					rows = append(rows, []string{t.ID, t.Name, t.Category, t.Description})
				}
				app.Out.Table([]string{"ID", "NAME", "CATEGORY", "DESCRIPTION"}, rows)
				app.Out.Info("%s", app.Out.Dim("Preview one: "+app.API.Base+"/api/v1/hosting/templates/<id>/preview/"))
			})
			return nil
		},
	})

	// create
	var cName, cSub, cTemplate, cDir, cRepo, cBranch, cFolder, cBuildCmd, cOutput, cInstall, cNode string
	var cBuild, cNoWait bool
	create := &cobra.Command{
		Use:   "create",
		Short: "Create a hosted site from a template, a folder or a GitHub repository",
		Long: `Create a site Webnak hosts, live at <subdomain>.webnak.site.

  webnak create --template portfolio --name "Ana Lima"
  webnak create --dir ./dist --name "Harbor Bakery" --subdomain harbor-bakery
  webnak create --repo you/site --name "My app" --build --command "npm run build" --output dist

With no source flag it asks (or lists templates). Waits until the site is live
unless --no-wait.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.needAuth(); err != nil {
				return err
			}
			o := app.Out
			source := ""
			switch {
			case cTemplate != "":
				source = "template"
			case cDir != "":
				source = "upload"
			case cRepo != "":
				source = "github"
			}
			if source == "" {
				i, err := o.Choose("Start from:", []string{"A template", "A folder on this computer", "A GitHub repository"})
				if err != nil {
					return fmt.Errorf("say where the site comes from: --template <id>, --dir <folder> or --repo <owner/name>")
				}
				source = []string{"template", "upload", "github"}[i]
				switch source {
				case "template":
					if cTemplate, err = o.Ask("Template ID (see `webnak templates`):", "portfolio"); err != nil {
						return err
					}
				case "upload":
					if cDir, err = o.Ask("Folder:", "."); err != nil {
						return err
					}
				case "github":
					if cRepo, err = o.Ask("Repository (owner/name):", ""); err != nil {
						return err
					}
				}
			}
			var err error
			if cName == "" {
				def := ""
				if cDir != "" {
					abs, _ := filepath.Abs(cDir)
					def = filepath.Base(abs)
				}
				if cName, err = o.Ask("Site name:", def); err != nil {
					return fmt.Errorf("give the site a name with --name")
				}
			}
			if cSub == "" {
				cSub = slugify(cName)
			}
			body := map[string]any{"name": cName, "subdomain": cSub, "workspaceId": app.Workspace, "source": source}
			switch source {
			case "template":
				body["template"] = cTemplate
			case "github":
				body["repo"], body["branch"], body["folder"] = cRepo, cBranch, cFolder
				if cBuild || cBuildCmd != "" {
					body["build"] = map[string]string{"command": cBuildCmd, "output": cOutput, "install": cInstall, "node": cNode}
				}
			}
			if source == "upload" {
				if _, err := os.Stat(filepath.Join(cDir, "index.html")); err != nil {
					return fmt.Errorf("%s has no index.html. Point --dir at the folder with your home page (for a build, the output folder such as dist)", cDir)
				}
			}
			var out struct {
				Site    Site    `json:"site"`
				Hosting Hosting `json:"hosting"`
			}
			if err := app.API.Do(app.Ctx(), "POST", "/api/v1/webnak/hosted", body, &out); err != nil {
				return err
			}
			o.Success("Created %s at %s", o.Bold(out.Site.Name), o.Cyan(out.Hosting.URL))
			if source == "upload" {
				if err := app.uploadDir(out.Site.ID, cDir); err != nil {
					return fmt.Errorf("the site was made, but uploading failed: %w (retry with `webnak deploy %s --site %s`)", err, cDir, out.Site.ID)
				}
				if err := app.publishDraft(out.Site.ID); err != nil {
					return err
				}
			}
			if cNoWait {
				o.Result(map[string]any{"site": out.Site, "hosting": out.Hosting}, nil)
				return nil
			}
			d, err := app.waitLive(out.Site.ID, "")
			if err != nil {
				return err
			}
			o.Result(map[string]any{"site": out.Site, "url": out.Hosting.URL, "deploy": d}, func() {
				o.Success("Live at %s", o.Cyan(out.Hosting.URL))
				o.Info("%s", o.Dim("Edit it by clicking: webnak edit "+out.Hosting.Subdomain))
			})
			return nil
		},
	}
	f := create.Flags()
	f.StringVar(&cName, "name", "", "site name")
	f.StringVar(&cSub, "subdomain", "", "address: <subdomain>.webnak.site (default: from the name)")
	f.StringVar(&cTemplate, "template", "", "template ID (see `webnak templates`)")
	f.StringVar(&cDir, "dir", "", "folder to upload (must contain index.html)")
	f.StringVar(&cRepo, "repo", "", "GitHub repository owner/name (connect GitHub first: `webnak github connect`)")
	f.StringVar(&cBranch, "branch", "", "branch (default: the repository's default)")
	f.StringVar(&cFolder, "folder", "", "folder in the repository that holds the site")
	f.BoolVar(&cBuild, "build", false, "build the repository (framework detected) before hosting")
	f.StringVar(&cBuildCmd, "command", "", "build command, e.g. \"npm run build\"")
	f.StringVar(&cOutput, "output", "", "build output folder, e.g. dist")
	f.StringVar(&cInstall, "install", "", "install command, e.g. \"npm ci\"")
	f.StringVar(&cNode, "node", "", "Node version for the build")
	f.BoolVar(&cNoWait, "no-wait", false, "return once created, without waiting for it to go live")
	root.AddCommand(create)

	// deploy
	var dSite, dName, dSub string
	var dNoWait, dCreate bool
	deploy := &cobra.Command{
		Use:   "deploy [folder]",
		Short: "Upload a folder and make it live (replaces the site's files)",
		Long: `Upload a folder (default: the current one) to a hosted site and put it live.
The folder must contain index.html (for a build, deploy its output, e.g. ./dist).

  webnak deploy ./dist --site harbor-bakery
  webnak deploy ./dist --create --name "Harbor Bakery"   # make the site if needed

For a GitHub site, ` + "`webnak deploy --site <site>`" + ` with no folder pulls and
builds the latest commit.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.needAuth(); err != nil {
				return err
			}
			o := app.Out
			dir := argOr(args, 0)
			var s *Site
			var err error
			if dSite != "" || !dCreate {
				s, err = app.pickSite(dSite)
				if err != nil && !(dCreate && strings.Contains(err.Error(), "no site matches")) {
					return err
				}
			}
			if s == nil {
				if dir == "" {
					dir = "."
				}
				if dName == "" {
					abs, _ := filepath.Abs(dir)
					dName = filepath.Base(abs)
				}
				cName, cSub, cDir = dName, dSub, dir
				return create.RunE(create, nil)
			}
			h, err := app.hosting(s.ID)
			if err != nil {
				return err
			}
			if dir == "" && h.Source == "github" {
				var out map[string]any
				if err := app.API.Do(app.Ctx(), "POST", "/api/v1/webnak/sites/"+api.Path(s.ID)+"/hosting/deploy", map[string]any{}, &out); err != nil {
					return err
				}
				o.Info("Deploying the latest commit of %s…", h.Repo)
			} else {
				if dir == "" {
					dir = "."
				}
				if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
					return fmt.Errorf("%s has no index.html. Deploy the folder with your home page (for a build, the output folder such as dist)", dir)
				}
				prev := ""
				if d := latestDeploy(h); d != nil {
					prev = d.ID
				}
				if err := app.uploadDir(s.ID, dir); err != nil {
					return err
				}
				if err := app.publishDraft(s.ID); err != nil {
					return err
				}
				_ = prev
			}
			if dNoWait {
				o.Result(map[string]any{"ok": true, "site": s.ID}, nil)
				return nil
			}
			d, err := app.waitLive(s.ID, "")
			if err != nil {
				return err
			}
			o.Result(map[string]any{"ok": true, "site": s, "url": h.URL, "deploy": d}, func() { o.Success("Live at %s", o.Cyan(h.URL)) })
			return nil
		},
	}
	deploy.Flags().StringVar(&dSite, "site", "", "site to deploy to (name, address or ID)")
	deploy.Flags().BoolVar(&dCreate, "create", false, "create the site if it doesn't exist")
	deploy.Flags().StringVar(&dName, "name", "", "name for a new site (with --create)")
	deploy.Flags().StringVar(&dSub, "subdomain", "", "subdomain for a new site (with --create)")
	deploy.Flags().BoolVar(&dNoWait, "no-wait", false, "don't wait for it to go live")
	root.AddCommand(deploy)

	// deploys
	deploysCmd := &cobra.Command{
		Use:   "deploys <site>",
		Short: "List a hosted site's deploys (and roll back, see logs)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.pickSite(argOr(args, 0))
			if err != nil {
				return err
			}
			h, err := app.hosting(s.ID)
			if err != nil {
				return err
			}
			latestDeploy(h)
			app.Out.Result(h.Deploys, func() {
				rows := [][]string{}
				for _, d := range h.Deploys {
					st := d.Status
					if d.ID == h.CurrentDeploy {
						st += " (current)"
					}
					rows = append(rows, []string{d.ID, st, d.CreatedAt.Local().Format("2 Jan 15:04"), fmt.Sprint(d.Files), short(d.Commit, 7), short(d.Error, 60)})
				}
				app.Out.Table([]string{"ID", "STATUS", "WHEN", "FILES", "COMMIT", "ERROR"}, rows)
			})
			return nil
		},
	}
	deploysCmd.AddCommand(&cobra.Command{
		Use:   "rollback <site> <deploy-id>",
		Short: "Put an earlier deploy back live",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			if err := app.API.Do(app.Ctx(), "POST", "/api/v1/webnak/sites/"+api.Path(s.ID)+"/hosting/rollback", map[string]string{"deployId": args[1]}, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true}, func() { app.Out.Success("Rolled back %s to %s.", s.Name, args[1]) })
			return nil
		},
	})
	deploysCmd.AddCommand(&cobra.Command{
		Use:   "logs <site> [deploy-id]",
		Short: "Show a deploy's build log (default: the latest)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			h, err := app.hosting(s.ID)
			if err != nil {
				return err
			}
			var d *Deploy
			if len(args) == 2 {
				for i := range h.Deploys {
					if h.Deploys[i].ID == args[1] {
						d = &h.Deploys[i]
					}
				}
			} else {
				d = latestDeploy(h)
			}
			if d == nil {
				return fmt.Errorf("%w: no such deploy", errNotFound)
			}
			app.Out.Result(d, func() {
				if d.Log == "" {
					app.Out.Print("No build log for this deploy (only built deploys have one).")
					return
				}
				fmt.Fprintln(app.Out.W, d.Log)
			})
			return nil
		},
	})
	root.AddCommand(deploysCmd)

	var dlOut string
	download := &cobra.Command{
		Use:   "download <site>",
		Short: "Download a hosted site as a zip, with all published edits written in",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.pickSite(argOr(args, 0))
			if err != nil {
				return err
			}
			res, err := app.API.Raw(app.Ctx(), "GET", "/api/v1/webnak/sites/"+api.Path(s.ID)+"/hosting/export", nil)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if dlOut == "" {
				dlOut = slugify(s.Name) + ".zip"
			}
			f, err := os.Create(dlOut)
			if err != nil {
				return err
			}
			n, err := io.Copy(f, res.Body)
			f.Close()
			if err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true, "file": dlOut, "bytes": n}, func() { app.Out.Success("Saved %s (%s).", dlOut, humanBytes(n)) })
			return nil
		},
	}
	download.Flags().StringVarP(&dlOut, "output", "o", "", "file to write (default: <site>.zip)")
	root.AddCommand(download)
}

// uploadDir sends a folder as the site's new files (into its draft).
func (a *App) uploadDir(siteID, dir string) error {
	type file struct {
		rel  string
		abs  string
		size int64
	}
	var files []file
	var total int64
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != dir && (strings.HasPrefix(name, ".") && name != ".well-known" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		files = append(files, file{filepath.ToSlash(rel), p, info.Size()})
		total += info.Size()
		return nil
	})
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("%s has no files to upload", dir)
	}
	// index.html first: the first part replaces the site and must have a home page.
	sort.SliceStable(files, func(i, j int) bool { return files[i].rel == "index.html" && files[j].rel != "index.html" })
	sp := a.Out.Spin(fmt.Sprintf("Uploading %d files (%s)", len(files), humanBytes(total)))
	defer sp.Stop()

	const part = 24 << 20
	// Part one: a zip that replaces everything, up to ~24 MB.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	i := 0
	for ; i < len(files); i++ {
		if i > 0 && int64(buf.Len())+files[i].size > part {
			break
		}
		w, err := zw.Create(files[i].rel)
		if err != nil {
			return err
		}
		f, err := os.Open(files[i].abs)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, f)
		f.Close()
		if err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := a.API.Send(a.Ctx(), "POST", "/api/v1/webnak/sites/"+api.Path(siteID)+"/hosting/files/zip?replace=1", &buf, "application/zip", nil); err != nil {
		return err
	}
	// The rest: batches added to the same draft.
	for i < len(files) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		size := int64(0)
		start := i
		for ; i < len(files) && (i == start || size+files[i].size < part) && i-start < 200; i++ {
			w, err := mw.CreateFormFile("path:"+files[i].rel, filepath.Base(files[i].rel))
			if err != nil {
				return err
			}
			f, err := os.Open(files[i].abs)
			if err != nil {
				return err
			}
			_, err = io.Copy(w, f)
			f.Close()
			if err != nil {
				return err
			}
			size += files[i].size
		}
		mw.Close()
		sp.Update(fmt.Sprintf("Uploading files %d of %d", i, len(files)))
		if err := a.API.Send(a.Ctx(), "POST", "/api/v1/webnak/sites/"+api.Path(siteID)+"/hosting/files/upload", &body, mw.FormDataContentType(), nil); err != nil {
			return err
		}
	}
	sp.Stop()
	a.Out.Success("Uploaded %d files (%s).", len(files), humanBytes(total))
	return nil
}

// publishDraft deploys the site's pending file changes.
func (a *App) publishDraft(siteID string) error {
	return a.API.Do(a.Ctx(), "POST", "/api/v1/webnak/sites/"+api.Path(siteID)+"/hosting/files/deploy", map[string]any{}, nil)
}

// waitLive follows the newest deploy until it is live or failed.
func (a *App) waitLive(siteID, after string) (*Deploy, error) {
	sp := a.Out.Spin("Going live")
	defer sp.Stop()
	deadline := time.Now().Add(25 * time.Minute)
	for time.Now().Before(deadline) {
		h, err := a.hosting(siteID)
		if err != nil {
			return nil, err
		}
		d := latestDeploy(h)
		if d != nil && d.ID != after {
			switch d.Status {
			case "live":
				// The deploy is live a moment before the server clears the
				// changes it took; wait for that, so the next command starts clean.
				if h.DraftChanges > 0 && time.Now().Before(deadline.Add(-24*time.Minute)) {
					sp.Update("Finishing up")
					break
				}
				sp.Stop()
				return d, nil
			case "failed":
				sp.Stop()
				msg := d.Error
				if d.Log != "" {
					msg += "\n\nBuild log (end):\n" + tail(d.Log, 30)
				}
				return d, fmt.Errorf("the deploy failed: %s", msg)
			case "building":
				label := "Going live"
				if d.Phase != "" {
					label = map[string]string{"queued": "Waiting for a build machine", "installing": "Installing packages", "building": "Building", "uploading": "Uploading the build"}[d.Phase]
				} else if d.Files > 0 {
					label = fmt.Sprintf("Copying files (%d)", d.Files)
				}
				sp.Update(label)
			}
		}
		select {
		case <-a.Ctx().Done():
			return nil, fmt.Errorf("stopped waiting (the deploy carries on; check with `webnak deploys`)")
		case <-time.After(2 * time.Second):
		}
	}
	return nil, fmt.Errorf("still not live after 25 minutes; check `webnak deploys`")
}

func argOr(args []string, i int) string {
	if len(args) > i {
		return args[i]
	}
	return ""
}

func short(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

func tail(s string, lines int) string {
	parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n")
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
