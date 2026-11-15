package cmd

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wolfigs/webnak-cli/internal/api"
	"github.com/wolfigs/webnak-cli/internal/ui"
)

func sitePath(id, rest string) string { return "/api/v1/webnak/sites/" + api.Path(id) + rest }

func init() {
	root.AddCommand(filesCommand(), domainsCommand(), editCommand(), contentCommand(), githubCommand(),
		accountCommand(), workspaceCommand(), planCommand())
}

// ---------------------------------------------------------------- files

func filesCommand() *cobra.Command {
	files := &cobra.Command{
		Use:   "files <site>",
		Short: "List and change a hosted site's files (changes go live with `files publish`)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.pickSite(argOr(args, 0))
			if err != nil {
				return err
			}
			var out struct {
				Files []map[string]any `json:"files"`
				Draft map[string]any   `json:"draft"`
			}
			if err := app.API.Do(app.Ctx(), "GET", sitePath(s.ID, "/hosting/files"), nil, &out); err != nil {
				return err
			}
			app.Out.Result(out, func() {
				rows := [][]string{}
				for _, f := range out.Files {
					st := strOr(f["state"], "")
					if st == "same" {
						st = ""
					}
					rows = append(rows, []string{fmt.Sprint(f["path"]), st})
				}
				app.Out.Table([]string{"PATH", "CHANGE"}, rows)
				if c := num(out.Draft["changes"]); c > 0 {
					app.Out.Info("%d change(s) not live yet. Put them live: %s", int(c), app.Out.Cyan("webnak files publish "+s.Ref()))
				}
			})
			return nil
		},
	}
	files.AddCommand(&cobra.Command{
		Use:   "get <site> <path> [local-file]",
		Short: "Print a file (or save it to local-file)",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			res, err := app.API.Raw(app.Ctx(), "GET", sitePath(s.ID, "/hosting/files/raw")+api.Q("path", args[1]), nil)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			var w io.Writer = os.Stdout
			if len(args) == 3 {
				f, err := os.Create(args[2])
				if err != nil {
					return err
				}
				defer f.Close()
				w = f
			}
			_, err = io.Copy(w, res.Body)
			return err
		},
	})
	files.AddCommand(&cobra.Command{
		Use:   "put <site> <path> <local-file|->",
		Short: "Upload one file to a path (- reads stdin)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			var r io.Reader = os.Stdin
			if args[2] != "-" {
				f, err := os.Open(args[2])
				if err != nil {
					return err
				}
				defer f.Close()
				r = f
			}
			if err := app.API.Send(app.Ctx(), "PUT", sitePath(s.ID, "/hosting/files/raw")+api.Q("path", args[1]), r, "application/octet-stream", nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true, "path": args[1]}, func() {
				app.Out.Success("Saved %s. Put it live: %s", args[1], app.Out.Cyan("webnak files publish "+s.Ref()))
			})
			return nil
		},
	})
	files.AddCommand(&cobra.Command{
		Use:   "rm <site> <path>...",
		Short: "Delete files (a folder with a trailing /)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			if err := app.API.Do(app.Ctx(), "POST", sitePath(s.ID, "/hosting/files/delete"), map[string]any{"paths": args[1:]}, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true}, func() { app.Out.Success("Deleted. Put it live: webnak files publish %s", s.Ref()) })
			return nil
		},
	})
	files.AddCommand(&cobra.Command{
		Use:   "mv <site> <from> <to>",
		Short: "Rename or move a file or folder",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			if err := app.API.Do(app.Ctx(), "POST", sitePath(s.ID, "/hosting/files/rename"), map[string]string{"from": args[1], "to": args[2]}, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true}, func() { app.Out.Success("Moved %s to %s.", args[1], args[2]) })
			return nil
		},
	})
	files.AddCommand(&cobra.Command{
		Use:   "discard <site>",
		Short: "Throw away file changes that aren't live yet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			if err := app.API.Do(app.Ctx(), "POST", sitePath(s.ID, "/hosting/files/discard"), map[string]any{}, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true}, func() { app.Out.Success("Discarded the changes.") })
			return nil
		},
	})
	var noWait bool
	pub := &cobra.Command{
		Use:     "publish <site>",
		Aliases: []string{"deploy"},
		Short:   "Put the file changes live (one new deploy)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			if err := app.publishDraft(s.ID); err != nil {
				return err
			}
			if noWait {
				app.Out.Result(map[string]any{"ok": true}, nil)
				return nil
			}
			d, err := app.waitLive(s.ID, "")
			if err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true, "deploy": d}, func() { app.Out.Success("Live at %s", app.Out.Cyan(s.Address())) })
			return nil
		},
	}
	pub.Flags().BoolVar(&noWait, "no-wait", false, "don't wait for it to go live")
	files.AddCommand(pub)
	return files
}

// ---------------------------------------------------------------- domains

func domainsCommand() *cobra.Command {
	dom := &cobra.Command{
		Use:     "domains <site>",
		Aliases: []string{"domain"},
		Short:   "Custom domains for a hosted site (Starter plan)",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.pickSite(argOr(args, 0))
			if err != nil {
				return err
			}
			var out map[string]any
			if err := app.API.Do(app.Ctx(), "GET", sitePath(s.ID, "/hosting/domains"), nil, &out); err != nil {
				return err
			}
			app.Out.Result(out, func() {
				list, _ := out["domains"].([]any)
				if len(list) == 0 {
					app.Out.Print("No custom domains. Add one: %s", app.Out.Cyan("webnak domains add "+s.Ref()+" yourdomain.com"))
					return
				}
				rows := [][]string{}
				for _, x := range list {
					d := x.(map[string]any)
					live := "waiting for DNS"
					if d["live"] == true {
						live = "live"
					}
					rows = append(rows, []string{fmt.Sprint(d["domain"]), live, strOr(d["redirectTo"], "")})
				}
				app.Out.Table([]string{"DOMAIN", "STATUS", "REDIRECTS TO"}, rows)
			})
			return nil
		},
	}
	var noPair bool
	var primary string
	add := &cobra.Command{
		Use:   "add <site> <domain>",
		Short: "Connect a domain (the bare domain and www together) and print the DNS records",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			var out map[string]any
			body := map[string]any{"domain": args[1], "withPair": !noPair, "primary": primary}
			if err := app.API.Do(app.Ctx(), "POST", sitePath(s.ID, "/hosting/domains"), body, &out); err != nil {
				var e *api.Error
				if asAPIError(err, &e) && e.Data["needsProof"] != nil {
					p := e.Data["needsProof"].(map[string]any)
					return fmt.Errorf("%s\n\n  %s  %s  %s", e.Message, p["type"], p["name"], p["value"])
				}
				return err
			}
			app.Out.Result(out, func() {
				app.Out.Success("Connecting %s. Add these records at your domain provider:", args[1])
				printRecords(out)
				app.Out.Info("%s", app.Out.Dim("Check progress: webnak domains check "+s.Name+" "+args[1]))
			})
			return nil
		},
	}
	add.Flags().BoolVar(&noPair, "only", false, "connect just this name, not its bare/www pair")
	add.Flags().StringVar(&primary, "primary", "", "which of the pair visitors land on: bare or www")
	dom.AddCommand(add)
	dom.AddCommand(&cobra.Command{
		Use:   "remove <site> <domain>",
		Short: "Disconnect a domain",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			if err := app.API.Do(app.Ctx(), "DELETE", sitePath(s.ID, "/hosting/domains/"+api.Path(args[1])), nil, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true}, func() { app.Out.Success("Disconnected %s.", args[1]) })
			return nil
		},
	})
	dom.AddCommand(&cobra.Command{
		Use:   "check <site> <domain>",
		Short: "Check a domain's DNS and HTTPS",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			var out map[string]any
			if err := app.API.Do(app.Ctx(), "GET", sitePath(s.ID, "/hosting/domain-check")+api.Q("domain", args[1]), nil, &out); err != nil {
				return err
			}
			app.Out.Result(out, func() {
				for k, v := range out {
					app.Out.Print("  %-12s %v", k+":", v)
				}
			})
			return nil
		},
	})
	dom.AddCommand(&cobra.Command{
		Use:   "primary <site> <domain>",
		Short: "Make a domain the one its pair redirects to",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			// The other half of the pair redirects here.
			other := "www." + args[1]
			if strings.HasPrefix(args[1], "www.") {
				other = strings.TrimPrefix(args[1], "www.")
			}
			if err := app.API.Do(app.Ctx(), "PATCH", sitePath(s.ID, "/hosting/domains/"+api.Path(other)), map[string]string{"redirectTo": args[1]}, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true}, func() { app.Out.Success("%s now redirects to %s.", other, args[1]) })
			return nil
		},
	})
	return dom
}

func printRecords(out map[string]any) {
	var recs []any
	if r, ok := out["records"].([]any); ok {
		recs = r
	}
	if added, ok := out["added"].([]any); ok {
		for _, a := range added {
			if m, ok := a.(map[string]any); ok {
				if r, ok := m["records"].([]any); ok {
					recs = append(recs, r...)
				}
			}
		}
	}
	rows := [][]string{}
	for _, x := range recs {
		r, _ := x.(map[string]any)
		rows = append(rows, []string{fmt.Sprint(r["type"]), strOr(r["host"], fmt.Sprint(r["name"])), fmt.Sprint(r["value"])})
	}
	app.Out.Table([]string{"TYPE", "NAME", "VALUE"}, rows)
}

// ---------------------------------------------------------------- editing

func editCommand() *cobra.Command {
	var page string
	c := &cobra.Command{
		Use:   "edit <site>",
		Short: "Open the visual editor on the live site in your browser",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.pickSite(argOr(args, 0))
			if err != nil {
				return err
			}
			if s.Address() == "" {
				return fmt.Errorf("%s has no address yet", s.Name)
			}
			var out struct {
				Token string `json:"token"`
			}
			if err := app.API.Do(app.Ctx(), "POST", sitePath(s.ID, "/edit-token"), map[string]any{}, &out); err != nil {
				return err
			}
			u := strings.TrimRight(s.Address(), "/") + "/" + strings.TrimLeft(page, "/") + "#webnak=" + out.Token
			if app.Out.JSON {
				app.Out.Result(map[string]any{"url": u}, nil)
				return nil
			}
			if ui.OpenBrowser(u) {
				app.Out.Success("Opened the editor on %s. Click anything to change it, then Publish.", s.Address())
			} else {
				app.Out.Print("Open this link to edit (it signs you in to the editor, keep it private):\n%s", u)
			}
			return nil
		},
	}
	c.Flags().StringVar(&page, "page", "/", "page path to open")
	return c
}

// contentCommand changes text, links and images without a browser, through the
// same edit API the visual editor uses.
func contentCommand() *cobra.Command {
	var page, sel, text, html, attr string
	content := &cobra.Command{Use: "content", Short: "Read and change page content without a browser"}
	editToken := func(s *Site) (*api.Client, error) {
		var out struct {
			Token string `json:"token"`
		}
		if err := app.API.Do(app.Ctx(), "POST", sitePath(s.ID, "/edit-token"), map[string]any{}, &out); err != nil {
			return nil, err
		}
		c := *app.API
		c.Token = out.Token
		return &c, nil
	}
	get := &cobra.Command{
		Use:   "get <site>",
		Short: "Show a page's edited elements (drafts and published)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			c, err := editToken(s)
			if err != nil {
				return err
			}
			var out map[string]any
			if err := c.Do(app.Ctx(), "GET", "/api/v1/webnak/edit/content"+api.Q("path", page), nil, &out); err != nil {
				return err
			}
			app.Out.Result(out, func() { app.Out.JSON = true; app.Out.Result(out, nil); app.Out.JSON = false })
			return nil
		},
	}
	get.Flags().StringVar(&page, "page", "/", "page path")
	set := &cobra.Command{
		Use:   "set <site>",
		Short: "Change an element: --selector '[data-webnak=\"hero-title\"]' --text \"New headline\"",
		Long: `Change one element on a page, as a draft. Then ` + "`webnak content publish`" + `.

  webnak content set my-site --selector '[data-webnak="hero-title"]' --text "Fresh bread daily"
  webnak content set my-site --selector '[data-webnak="hero-image"]' --attr src=https://…/photo.jpg

Templates name every editable element with data-webnak; ` + "`webnak files get <site> index.html`" + ` shows them.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if sel == "" {
				return fmt.Errorf("say which element with --selector")
			}
			cc := map[string]any{}
			if cmd.Flags().Changed("text") {
				cc["text"] = text
			}
			if cmd.Flags().Changed("html") {
				cc["html"] = html
			}
			if attr != "" {
				k, v, ok := strings.Cut(attr, "=")
				if !ok {
					return fmt.Errorf("--attr takes name=value")
				}
				cc["attrs"] = map[string]string{k: v}
			}
			if len(cc) == 0 {
				return fmt.Errorf("say what to change: --text, --html or --attr name=value")
			}
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			c, err := editToken(s)
			if err != nil {
				return err
			}
			if err := c.Do(app.Ctx(), "PUT", "/api/v1/webnak/edit/content", map[string]any{"path": page, "selector": sel, "content": cc}, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true, "draft": true}, func() {
				app.Out.Success("Saved as a draft. Put it live: %s", app.Out.Cyan("webnak content publish "+s.Ref()+" --page "+page))
			})
			return nil
		},
	}
	set.Flags().StringVar(&page, "page", "/", "page path")
	set.Flags().StringVar(&sel, "selector", "", "the element, e.g. [data-webnak=\"hero-title\"]")
	set.Flags().StringVar(&text, "text", "", "new text")
	set.Flags().StringVar(&html, "html", "", "new inner HTML (sanitised)")
	set.Flags().StringVar(&attr, "attr", "", "set an attribute, e.g. src=https://… or href=/contact")
	for _, verb := range []struct{ use, short, path, done string }{
		{"publish", "Put a page's draft edits live", "/api/v1/webnak/edit/publish", "Published."},
		{"discard", "Throw away a page's draft edits", "/api/v1/webnak/edit/discard", "Discarded the drafts."},
	} {
		v := verb
		var p string
		c := &cobra.Command{
			Use:   v.use + " <site>",
			Short: v.short,
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				s, err := app.findSite(args[0])
				if err != nil {
					return err
				}
				c, err := editToken(s)
				if err != nil {
					return err
				}
				var out map[string]any
				if err := c.Do(app.Ctx(), "POST", v.path, map[string]string{"path": p}, &out); err != nil {
					return err
				}
				app.Out.Result(out, func() {
					if st, _ := out["status"].(string); st != "" {
						app.Out.Success("%s", st)
						return
					}
					app.Out.Success("%s", v.done)
				})
				return nil
			},
		}
		c.Flags().StringVar(&p, "page", "/", "page path")
		content.AddCommand(c)
	}
	content.AddCommand(get, set)
	content.AddCommand(&cobra.Command{
		Use:   "pages <site>",
		Short: "List the pages that have edits",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			var out []map[string]any
			if err := app.API.Do(app.Ctx(), "GET", sitePath(s.ID, "/pages"), nil, &out); err != nil {
				return err
			}
			app.Out.Result(out, func() {
				rows := [][]string{}
				for _, p := range out {
					rows = append(rows, []string{fmt.Sprint(p["path"]), fmt.Sprint(p["id"])})
				}
				app.Out.Table([]string{"PATH", "ID"}, rows)
			})
			return nil
		},
	})
	return content
}

// ---------------------------------------------------------------- github

func githubCommand() *cobra.Command {
	gh := &cobra.Command{Use: "github", Short: "Connect GitHub and host repositories"}
	gh.AddCommand(&cobra.Command{
		Use:   "connect",
		Short: "Install the Webnak GitHub app on your repositories (in the browser)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.needAuth(); err != nil {
				return err
			}
			var out struct {
				URL string `json:"url"`
			}
			if err := app.API.Do(app.Ctx(), "POST", "/api/v1/github/connect", map[string]any{}, &out); err != nil {
				return err
			}
			if app.Out.JSON {
				app.Out.Result(out, nil)
				return nil
			}
			if !ui.OpenBrowser(out.URL) {
				app.Out.Print("Open this link to connect GitHub:\n%s", out.URL)
			}
			sp := app.Out.Spin("Waiting for GitHub (choose the repositories, then come back)")
			defer sp.Stop()
			for i := 0; i < 150; i++ {
				time.Sleep(2 * time.Second)
				var repos map[string]any
				if err := app.API.Do(app.Ctx(), "GET", "/api/v1/github/repos", nil, &repos); err == nil {
					if l, ok := repos["repos"].([]any); ok && len(l) > 0 {
						sp.Stop()
						app.Out.Success("GitHub connected (%d repositories). Host one: webnak create --repo owner/name --name …", len(l))
						return nil
					}
				}
			}
			return fmt.Errorf("didn't see GitHub connect in 5 minutes; run `webnak github repos` to check")
		},
	})
	gh.AddCommand(&cobra.Command{
		Use:   "repos",
		Short: "List the repositories Webnak can reach",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.needAuth(); err != nil {
				return err
			}
			var out map[string]any
			if err := app.API.Do(app.Ctx(), "GET", "/api/v1/github/repos", nil, &out); err != nil {
				return err
			}
			app.Out.Result(out, func() {
				l, _ := out["repos"].([]any)
				if len(l) == 0 {
					app.Out.Print("No repositories yet. Run `webnak github connect`.")
					return
				}
				rows := [][]string{}
				for _, x := range l {
					r, _ := x.(map[string]any)
					rows = append(rows, []string{strOr(r["fullName"], fmt.Sprint(r["name"])), strOr(r["defaultBranch"], ""), map[bool]string{true: "private", false: "public"}[r["private"] == true]})
				}
				app.Out.Table([]string{"REPOSITORY", "BRANCH", "VISIBILITY"}, rows)
			})
			return nil
		},
	})
	var bCmd, bOut, bInstall, bNode, bFramework string
	build := &cobra.Command{
		Use:   "build <site>",
		Short: "Change how a GitHub site is built (command, output folder, Node)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := app.findSite(args[0])
			if err != nil {
				return err
			}
			body := map[string]string{"framework": bFramework, "command": bCmd, "output": bOut, "install": bInstall, "node": bNode}
			var out map[string]any
			if err := app.API.Do(app.Ctx(), "PUT", sitePath(s.ID, "/hosting/build"), body, &out); err != nil {
				return err
			}
			app.Out.Result(out, func() { app.Out.Success("Build settings saved. Deploy with: webnak deploy --site %s", s.Ref()) })
			return nil
		},
	}
	build.Flags().StringVar(&bCmd, "command", "", "build command")
	build.Flags().StringVar(&bOut, "output", "", "output folder")
	build.Flags().StringVar(&bInstall, "install", "", "install command")
	build.Flags().StringVar(&bNode, "node", "", "Node version")
	build.Flags().StringVar(&bFramework, "framework", "", "framework id (next, vite, astro, …)")
	gh.AddCommand(build)
	return gh
}

// ---------------------------------------------------------------- account

func accountCommand() *cobra.Command {
	acc := &cobra.Command{Use: "account", Short: "Your profile, password, two-factor and sessions"}
	var name string
	prof := &cobra.Command{
		Use:   "profile",
		Short: "Show your profile, or change your name with --name",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.needAuth(); err != nil {
				return err
			}
			var u map[string]any
			var err error
			if name != "" {
				err = app.API.Do(app.Ctx(), "PATCH", "/api/v1/me", map[string]string{"name": name}, &u)
			} else {
				err = app.API.Do(app.Ctx(), "GET", "/api/v1/me", nil, &u)
			}
			if err != nil {
				return err
			}
			app.Out.Result(u, func() {
				app.Out.Print("%s <%s>  role: %v  verified: %v", u["name"], u["email"], u["role"], u["emailVerified"])
			})
			return nil
		},
	}
	prof.Flags().StringVar(&name, "name", "", "new display name")
	acc.AddCommand(prof)
	acc.AddCommand(&cobra.Command{
		Use:   "password",
		Short: "Change your password",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.needAuth(); err != nil {
				return err
			}
			cur, err := app.Out.Secret("Current password:")
			if err != nil {
				return err
			}
			nw, err := app.Out.Secret("New password:")
			if err != nil {
				return err
			}
			if err := app.API.Do(app.Ctx(), "POST", "/api/v1/me/password", map[string]string{"currentPassword": cur, "newPassword": nw}, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true}, func() { app.Out.Success("Password changed.") })
			return nil
		},
	})
	tfa := &cobra.Command{
		Use:   "2fa",
		Short: "Two-factor sign-in: status, add an authenticator app, email codes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.needAuth(); err != nil {
				return err
			}
			var out map[string]any
			if err := app.API.Do(app.Ctx(), "GET", "/api/v1/me/2fa", nil, &out); err != nil {
				return err
			}
			app.Out.Result(out, func() {
				for k, v := range out {
					app.Out.Print("  %-16s %v", k+":", v)
				}
			})
			return nil
		},
	}
	tfa.AddCommand(&cobra.Command{
		Use:   "totp",
		Short: "Add an authenticator app (prints the setup key and link)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.needAuth(); err != nil {
				return err
			}
			var setup struct {
				Secret     string `json:"secret"`
				OtpauthURI string `json:"otpauthUri"`
			}
			if err := app.API.Do(app.Ctx(), "POST", "/api/v1/me/2fa/totp/setup", map[string]any{}, &setup); err != nil {
				return err
			}
			o := app.Out
			o.Info("Add this to your authenticator app:")
			o.Info("  Key:  %s", o.Bold(setup.Secret))
			o.Info("  Link: %s", setup.OtpauthURI)
			code, err := o.Ask("Enter the 6-digit code it shows:", "")
			if err != nil {
				return err
			}
			var out map[string]any
			if err := app.API.Do(app.Ctx(), "POST", "/api/v1/me/2fa/totp", map[string]string{"secret": setup.Secret, "code": code, "name": "Authenticator (CLI)"}, &out); err != nil {
				return err
			}
			o.Result(out, func() {
				o.Success("Authenticator added.")
				if rc, ok := out["recoveryCodes"].([]any); ok && len(rc) > 0 {
					o.Print("Recovery codes (save them somewhere safe; each works once):")
					for _, c := range rc {
						o.Print("  %v", c)
					}
				}
			})
			return nil
		},
	})
	for _, t := range []struct{ use, short, path string }{
		{"email-on", "Turn on email codes for signing in", "/api/v1/me/2fa/email/enable"},
		{"email-off", "Turn off email codes", "/api/v1/me/2fa/email/disable"},
		{"recovery", "Make new recovery codes (old ones stop working)", "/api/v1/me/2fa/recovery"},
	} {
		t := t
		tfa.AddCommand(&cobra.Command{
			Use:   t.use,
			Short: t.short,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.needAuth(); err != nil {
					return err
				}
				var out map[string]any
				if err := app.API.Do(app.Ctx(), "POST", t.path, map[string]any{}, &out); err != nil {
					return err
				}
				app.Out.Result(out, func() {
					app.Out.Success("Done.")
					if rc, ok := out["recoveryCodes"].([]any); ok {
						for _, c := range rc {
							app.Out.Print("  %v", c)
						}
					}
				})
				return nil
			},
		})
	}
	acc.AddCommand(tfa)
	sess := &cobra.Command{
		Use:   "sessions",
		Short: "List where you're signed in (revoke with `sessions revoke <id>`)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.needAuth(); err != nil {
				return err
			}
			var raw any
			if err := app.API.Do(app.Ctx(), "GET", "/api/v1/me/sessions", nil, &raw); err != nil {
				return err
			}
			out := asList(raw, "sessions")
			app.Out.Result(out, func() {
				rows := [][]string{}
				for _, s := range out {
					cur := ""
					if s["current"] == true {
						cur = "this one"
					}
					rows = append(rows, []string{short(fmt.Sprint(s["userAgent"]), 50), fmt.Sprint(s["ip"]), fmt.Sprint(s["lastSeen"]), cur, short(fmt.Sprint(s["id"]), 14)})
				}
				app.Out.Table([]string{"DEVICE", "IP", "LAST SEEN", "", "ID"}, rows)
			})
			return nil
		},
	}
	sess.AddCommand(&cobra.Command{
		Use:   "revoke <id>",
		Short: "Sign out one session (the ID from `account sessions`, or its first characters)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw any
			if err := app.API.Do(app.Ctx(), "GET", "/api/v1/me/sessions", nil, &raw); err != nil {
				return err
			}
			out := asList(raw, "sessions")
			id := ""
			for _, s := range out {
				if sid := fmt.Sprint(s["id"]); strings.HasPrefix(sid, strings.TrimSuffix(args[0], "…")) {
					id = sid
				}
			}
			if id == "" {
				return fmt.Errorf("%w: no session %s", errNotFound, args[0])
			}
			if err := app.API.Do(app.Ctx(), "DELETE", "/api/v1/me/sessions/"+url.PathEscape(id), nil, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true}, func() { app.Out.Success("Signed out that session.") })
			return nil
		},
	})
	acc.AddCommand(sess)
	return acc
}

// ---------------------------------------------------------------- workspaces and plan

func workspaceCommand() *cobra.Command {
	ws := &cobra.Command{
		Use:     "workspaces",
		Aliases: []string{"workspace", "ws"},
		Short:   "List workspaces (and choose the one commands act in)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.needAuth(); err != nil {
				return err
			}
			var out any
			if err := app.API.Do(app.Ctx(), "GET", "/api/v1/workspaces", nil, &out); err != nil {
				return err
			}
			list := asList(out, "workspaces")
			app.Out.Result(out, func() {
				rows := [][]string{}
				for _, w := range list {
					cur := ""
					if fmt.Sprint(w["id"]) == app.Workspace {
						cur = "in use"
					}
					rows = append(rows, []string{fmt.Sprint(w["name"]), strOr(w["planName"], strOr(w["plan"], "")), fmt.Sprint(w["id"]), cur})
				}
				app.Out.Table([]string{"NAME", "PLAN", "ID", ""}, rows)
			})
			return nil
		},
	}
	ws.AddCommand(&cobra.Command{
		Use:   "use <id-or-name>",
		Short: "Act in this workspace from now on (`use personal` to go back)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "personal" || args[0] == "-" {
				app.Cfg.Workspace = ""
			} else {
				var out any
				if err := app.API.Do(app.Ctx(), "GET", "/api/v1/workspaces", nil, &out); err != nil {
					return err
				}
				found := ""
				for _, w := range asList(out, "workspaces") {
					if fmt.Sprint(w["id"]) == args[0] || strings.EqualFold(fmt.Sprint(w["name"]), args[0]) {
						found = fmt.Sprint(w["id"])
					}
				}
				if found == "" {
					return fmt.Errorf("%w: no workspace %q", errNotFound, args[0])
				}
				app.Cfg.Workspace = found
			}
			if err := saveCfg(); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true, "workspace": app.Cfg.Workspace}, func() { app.Out.Success("Using workspace %s.", firstNonEmpty(app.Cfg.Workspace, "personal")) })
			return nil
		},
	})
	return ws
}

func planCommand() *cobra.Command {
	plan := &cobra.Command{
		Use:   "plan",
		Short: "Show your plan and usage (`plan upgrade` opens checkout)",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := app.workspaceID()
			if err != nil {
				return err
			}
			var usage, sub map[string]any
			_ = app.API.Do(app.Ctx(), "GET", "/api/v1/workspaces/"+api.Path(id)+"/usage", nil, &usage)
			_ = app.API.Do(app.Ctx(), "GET", "/api/v1/workspaces/"+api.Path(id)+"/subscription", nil, &sub)
			out := map[string]any{"workspace": id, "usage": usage, "subscription": sub}
			app.Out.Result(out, func() {
				app.Out.JSON = true
				app.Out.Result(out, nil)
				app.Out.JSON = false
			})
			return nil
		},
	}
	plan.AddCommand(&cobra.Command{
		Use:   "upgrade",
		Short: "Open the plan page to upgrade (payment happens in the browser)",
		RunE: func(cmd *cobra.Command, args []string) error {
			u := DashboardURL() + "/webnak/plan"
			if !ui.OpenBrowser(u) {
				app.Out.Print("%s", u)
			}
			app.Out.Result(map[string]any{"url": u}, nil)
			return nil
		},
	})
	return plan
}

// workspaceID is the workspace in use, or the personal one.
func (a *App) workspaceID() (string, error) {
	if err := a.needAuth(); err != nil {
		return "", err
	}
	if a.Workspace != "" {
		return a.Workspace, nil
	}
	var out any
	if err := a.API.Do(a.Ctx(), "GET", "/api/v1/workspaces", nil, &out); err != nil {
		return "", err
	}
	list := asList(out, "workspaces")
	for _, w := range list {
		if w["personal"] == true || w["kind"] == "personal" {
			return fmt.Sprint(w["id"]), nil
		}
	}
	if len(list) > 0 {
		return fmt.Sprint(list[0]["id"]), nil
	}
	return "", fmt.Errorf("no workspace found")
}

// ---------------------------------------------------------------- helpers

func saveCfg() error { return configSave() }

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

func strOr(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

func asList(v any, key string) []map[string]any {
	var raw []any
	switch t := v.(type) {
	case []any:
		raw = t
	case map[string]any:
		raw, _ = t[key].([]any)
	}
	out := []map[string]any{}
	for _, x := range raw {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
