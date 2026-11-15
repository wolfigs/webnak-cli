// Package cmd holds the webnak commands.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wolfigs/webnak-cli/internal/api"
	"github.com/wolfigs/webnak-cli/internal/config"
	"github.com/wolfigs/webnak-cli/internal/ui"
)

// Version is set at build time.
var Version = "dev"

// Exit codes, so scripts and agents can tell failures apart.
const (
	ExitOK       = 0
	ExitError    = 1
	ExitUsage    = 2
	ExitAuth     = 3
	ExitNotFound = 4
)

// App is what every command works with.
type App struct {
	Cfg       *config.Config
	Out       *ui.Out
	API       *api.Client
	Workspace string
	Yes       bool
	ctx       context.Context
}

func (a *App) Ctx() context.Context { return a.ctx }

// DashboardURL is the web dashboard, for links and browser sign-in.
func DashboardURL() string {
	if v := os.Getenv("WEBNAK_DASHBOARD"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://cloud.wolfigs.com"
}

// errAuth is returned when a command needs a sign-in.
var errAuth = errors.New("not signed in. Run `webnak login` (or set WEBNAK_TOKEN)")

// needAuth fails early, clearly, when there is no token.
func (a *App) needAuth() error {
	if a.API.Token == "" {
		return errAuth
	}
	return nil
}

var app = &App{Out: ui.New()}

var flags struct {
	json, noInput, quiet, yes bool
	api, token, workspace     string
}

var root = &cobra.Command{
	Use:   "webnak",
	Short: "Host and edit websites with Webnak, from your terminal",
	Long: `Webnak puts websites online and lets anyone edit them by clicking.
This CLI does everything the dashboard does: sign up and sign in, create sites
from templates, your files or GitHub, deploy, edit content, connect domains,
and more. Every command also works without prompts (flags, --json) for
scripts, CI and AI agents such as Claude Code.

Sign in:      webnak login
Make a site:  webnak create --template portfolio --name "My site"
Deploy files: webnak deploy ./dist --site my-site
Docs:         https://webnak.dev/docs`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("reading your Webnak settings: %w", err)
		}
		app.Cfg = cfg
		app.Out.JSON = flags.json
		app.Out.Quiet = flags.quiet
		app.Out.NoInput = flags.noInput || os.Getenv("CI") != "" || os.Getenv("WEBNAK_NO_INPUT") != ""
		app.Yes = flags.yes
		base := firstNonEmpty(flags.api, os.Getenv("WEBNAK_API"), cfg.API, api.DefaultBase)
		token := firstNonEmpty(flags.token, os.Getenv("WEBNAK_TOKEN"), cfg.Token)
		app.API = api.New(base, token, Version)
		app.Workspace = firstNonEmpty(flags.workspace, os.Getenv("WEBNAK_WORKSPACE"), cfg.Workspace)
		return nil
	},
}

func init() {
	pf := root.PersistentFlags()
	pf.BoolVar(&flags.json, "json", false, "print machine-readable JSON on stdout")
	pf.BoolVar(&flags.noInput, "no-input", false, "never prompt (also on when CI or WEBNAK_NO_INPUT is set)")
	pf.BoolVarP(&flags.quiet, "quiet", "q", false, "print only results")
	pf.BoolVarP(&flags.yes, "yes", "y", false, "answer yes to confirmations")
	pf.StringVar(&flags.api, "api", "", "API base URL (env WEBNAK_API)")
	pf.StringVar(&flags.token, "token", "", "sign-in token (env WEBNAK_TOKEN)")
	pf.StringVar(&flags.workspace, "workspace", "", "workspace ID to act in (env WEBNAK_WORKSPACE)")
	root.Version = Version
	root.SetVersionTemplate("webnak {{.Version}}\n")
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	app.ctx = ctx
	root.Version = Version
	err := root.ExecuteContext(ctx)
	if err == nil {
		return ExitOK
	}
	code := ExitError
	switch {
	case errors.Is(err, errAuth) || api.IsStatus(err, 401):
		code = ExitAuth
	case api.IsStatus(err, 404) || errors.Is(err, errNotFound):
		code = ExitNotFound
	case errors.Is(err, ui.ErrNoInput) || strings.Contains(err.Error(), "unknown flag") || strings.Contains(err.Error(), "unknown command") || strings.Contains(err.Error(), "accepts "):
		code = ExitUsage
	}
	msg := err.Error()
	if api.IsStatus(err, 401) && app.API != nil && app.API.Token != "" {
		msg = "your sign-in has expired or was revoked. Run `webnak login` again"
	}
	if app.Out.JSON {
		app.Out.Result(map[string]any{"ok": false, "error": msg, "exitCode": code}, nil)
	} else {
		fmt.Fprintf(os.Stderr, "%s %s\n", app.Out.Red("error:"), msg)
	}
	return code
}

func configSave() error { return config.Save(app.Cfg) }
