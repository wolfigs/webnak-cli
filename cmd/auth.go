package cmd

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wolfigs/webnak-cli/internal/api"
	"github.com/wolfigs/webnak-cli/internal/config"
	"github.com/wolfigs/webnak-cli/internal/ui"
)

// User is the signed-in account.
type User struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	Role          string `json:"role"`
	EmailVerified bool   `json:"emailVerified"`
}

func hostName() string {
	h, _ := os.Hostname()
	if h == "" {
		h = "this computer"
	}
	return h
}

// saveSignIn keeps the token (and the API it belongs to) for next time.
func (a *App) saveSignIn(token string, u *User) error {
	a.Cfg.Token = token
	if u != nil {
		a.Cfg.Email = u.Email
	}
	if a.API.Base != api.DefaultBase {
		a.Cfg.API = a.API.Base
	}
	a.API.Token = token
	return config.Save(a.Cfg)
}

func init() {
	var usePassword, noBrowser bool
	var email, password, code, tokenArg, name string

	login := &cobra.Command{
		Use:   "login",
		Short: "Sign in (in your browser by default)",
		Long: `Sign in to Webnak.

By default this opens your browser: sign in there (passkeys and two-factor
work as usual), approve the request, and the terminal carries on by itself.

  --password   sign in here with email and password instead (asks for a
               two-factor code if your account has one)
  --with-token save a token you already have (for example from
               ` + "`webnak token create`" + ` on another machine)

For CI and agents, skip login entirely and set WEBNAK_TOKEN.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if tokenArg != "" {
				return loginWithToken(tokenArg)
			}
			if usePassword || email != "" || password != "" {
				return loginWithPassword(email, password, code, name)
			}
			return loginInBrowser(name, noBrowser)
		},
	}
	login.Flags().BoolVar(&usePassword, "password", false, "sign in here with email and password")
	login.Flags().StringVar(&email, "email", "", "email (implies --password)")
	login.Flags().StringVar(&password, "pass", "", "password (or set WEBNAK_PASSWORD); prefer the prompt")
	login.Flags().StringVar(&code, "code", "", "two-factor code (authenticator, email or recovery)")
	login.Flags().StringVar(&tokenArg, "with-token", "", "save this sign-in token")
	login.Flags().StringVar(&name, "name", "", "name for this sign-in in Settings → Sessions (default: this computer's name)")
	login.Flags().BoolVar(&noBrowser, "no-browser", false, "print the link instead of opening the browser")
	root.AddCommand(login)

	root.AddCommand(&cobra.Command{
		Use:   "logout",
		Short: "Sign out and revoke this computer's token",
		RunE: func(cmd *cobra.Command, args []string) error {
			if app.API.Token != "" && strings.HasPrefix(app.API.Token, "wnk_") {
				_ = app.API.Do(app.Ctx(), "POST", "/api/v1/cli/logout", nil, nil)
			}
			app.Cfg.Token, app.Cfg.Email = "", ""
			if err := config.Save(app.Cfg); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true, "signedOut": true}, func() { app.Out.Success("Signed out.") })
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use:     "whoami",
		Short:   "Show who you are signed in as",
		Aliases: []string{"me"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.needAuth(); err != nil {
				return err
			}
			var u map[string]any
			if err := app.API.Do(app.Ctx(), "GET", "/api/v1/me", nil, &u); err != nil {
				return err
			}
			app.Out.Result(u, func() {
				app.Out.Print("%s %s", app.Out.Bold(fmt.Sprint(u["name"])), app.Out.Dim("<"+fmt.Sprint(u["email"])+">"))
				app.Out.Print("%s %s", app.Out.Dim("API:"), app.API.Base)
			})
			return nil
		},
	})

	var suEmail, suName, suPass string
	signup := &cobra.Command{
		Use:     "signup",
		Aliases: []string{"register"},
		Short:   "Create a Webnak account",
		Long: `Create a Webnak account. You'll get an email with a link to confirm your
address; click it (or pass the link to ` + "`webnak verify-email`" + `), then sign in.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			o := app.Out
			var err error
			if suEmail == "" {
				if suEmail, err = o.Ask("Email:", ""); err != nil {
					return err
				}
			}
			if suName == "" {
				if suName, err = o.Ask("Your name:", ""); err != nil {
					return err
				}
			}
			if suPass == "" {
				suPass = os.Getenv("WEBNAK_PASSWORD")
			}
			if suPass == "" {
				if suPass, err = o.Secret("Choose a password (8+ characters):"); err != nil {
					return err
				}
				again, err := o.Secret("Type it again:")
				if err != nil {
					return err
				}
				if again != suPass {
					return fmt.Errorf("the passwords didn't match")
				}
			}
			var out map[string]any
			if err := app.API.Do(app.Ctx(), "POST", "/api/v1/auth/register", map[string]string{"email": suEmail, "password": suPass, "name": suName}, &out); err != nil {
				return err
			}
			o.Result(map[string]any{"ok": true, "email": suEmail, "next": "verify your email, then run `webnak login`"}, func() {
				o.Success("Account created for %s.", o.Bold(suEmail))
				o.Info("Check your inbox for a confirmation link and click it. Then run %s.", o.Cyan("webnak login"))
				o.Info("%s", o.Dim("(Or paste the link into `webnak verify-email <link>`.)"))
			})
			return nil
		},
	}
	signup.Flags().StringVar(&suEmail, "email", "", "email address")
	signup.Flags().StringVar(&suName, "name", "", "your name")
	signup.Flags().StringVar(&suPass, "pass", "", "password (or set WEBNAK_PASSWORD); prefer the prompt")
	root.AddCommand(signup)

	root.AddCommand(&cobra.Command{
		Use:   "verify-email <link-or-token>",
		Short: "Confirm your email with the link from the confirmation email",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tok := args[0]
			if u, err := url.Parse(tok); err == nil && u.Query().Get("token") != "" {
				tok = u.Query().Get("token")
			}
			if err := app.API.Do(app.Ctx(), "POST", "/api/v1/auth/verify-email", map[string]string{"token": tok}, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true, "verified": true}, func() {
				app.Out.Success("Email confirmed. Run %s to sign in.", app.Out.Cyan("webnak login"))
			})
			return nil
		},
	})

	var resendEmail string
	resend := &cobra.Command{
		Use:   "resend-verification",
		Short: "Send the email confirmation link again",
		RunE: func(cmd *cobra.Command, args []string) error {
			e := resendEmail
			var err error
			if e == "" {
				if e, err = app.Out.Ask("Email:", app.Cfg.Email); err != nil {
					return err
				}
			}
			if err := app.API.Do(app.Ctx(), "POST", "/api/v1/auth/resend-verification", map[string]string{"email": e}, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true}, func() { app.Out.Success("If %s has an unconfirmed account, a new link is on its way.", e) })
			return nil
		},
	}
	resend.Flags().StringVar(&resendEmail, "email", "", "email address")
	root.AddCommand(resend)

	var fpEmail string
	forgot := &cobra.Command{
		Use:   "forgot-password",
		Short: "Email yourself a password reset link",
		RunE: func(cmd *cobra.Command, args []string) error {
			e := fpEmail
			var err error
			if e == "" {
				if e, err = app.Out.Ask("Email:", app.Cfg.Email); err != nil {
					return err
				}
			}
			if err := app.API.Do(app.Ctx(), "POST", "/api/v1/auth/forgot", map[string]string{"email": e}, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true}, func() {
				app.Out.Success("If %s has an account, a reset link is on its way. Then run `webnak reset-password <link>`.", e)
			})
			return nil
		},
	}
	forgot.Flags().StringVar(&fpEmail, "email", "", "email address")
	root.AddCommand(forgot)

	var rpPass string
	reset := &cobra.Command{
		Use:   "reset-password <link-or-token>",
		Short: "Choose a new password with the link from the reset email",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tok := args[0]
			if u, err := url.Parse(tok); err == nil && u.Query().Get("token") != "" {
				tok = u.Query().Get("token")
			}
			p := firstNonEmpty(rpPass, os.Getenv("WEBNAK_PASSWORD"))
			var err error
			if p == "" {
				if p, err = app.Out.Secret("New password:"); err != nil {
					return err
				}
			}
			if err := app.API.Do(app.Ctx(), "POST", "/api/v1/auth/reset", map[string]string{"token": tok, "password": p}, nil); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"ok": true}, func() { app.Out.Success("Password changed. Run `webnak login`.") })
			return nil
		},
	}
	reset.Flags().StringVar(&rpPass, "pass", "", "new password (or set WEBNAK_PASSWORD)")
	root.AddCommand(reset)

	var tkName string
	token := &cobra.Command{Use: "token", Short: "Make sign-in tokens for CI and agents"}
	tokCreate := &cobra.Command{
		Use:   "create",
		Short: "Create a long-lived token (prints it once)",
		Long: `Create a sign-in token for CI or an AI agent. Use it as WEBNAK_TOKEN.
It lasts a year and appears in Settings → Sessions, where you can revoke it.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.needAuth(); err != nil {
				return err
			}
			var out struct {
				Token string `json:"token"`
			}
			if err := app.API.Do(app.Ctx(), "POST", "/api/v1/cli/tokens", map[string]string{"name": firstNonEmpty(tkName, "token")}, &out); err != nil {
				return err
			}
			app.Out.Result(map[string]any{"token": out.Token}, func() {
				fmt.Fprintln(app.Out.W, out.Token)
				app.Out.Info("%s", app.Out.Dim("Keep it secret. Use it as WEBNAK_TOKEN. Revoke it in Settings → Sessions."))
			})
			return nil
		},
	}
	tokCreate.Flags().StringVar(&tkName, "name", "", "what the token is for (shown in Settings → Sessions)")
	token.AddCommand(tokCreate)
	root.AddCommand(token)
}

func loginWithToken(tok string) error {
	app.API.Token = tok
	var u User
	if err := app.API.Do(app.Ctx(), "GET", "/api/v1/me", nil, &u); err != nil {
		return fmt.Errorf("that token didn't work: %w", err)
	}
	if err := app.saveSignIn(tok, &u); err != nil {
		return err
	}
	app.Out.Result(map[string]any{"ok": true, "user": u}, func() { app.Out.Success("Signed in as %s.", app.Out.Bold(u.Email)) })
	return nil
}

func loginInBrowser(name string, noBrowser bool) error {
	o := app.Out
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	secret := hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(secret))
	verifier := hex.EncodeToString(sum[:])
	code := strings.ToUpper(verifier[:4] + "-" + verifier[4:8])
	device := firstNonEmpty(name, hostName())
	link := DashboardURL() + "/cli/login?" + url.Values{"v": {verifier}, "name": {device}}.Encode()

	if o.JSON {
		// Agents: print the link at once so a person can open it, then wait.
		o.Result(map[string]any{"status": "waiting", "url": link, "code": code, "expiresInSeconds": 600}, nil)
	} else {
		o.Info("")
		o.Info("  %s", o.Bold("Sign in to Webnak in your browser"))
		o.Info("  Code: %s  %s", o.Orange(o.Bold(code)), o.Dim("(check it matches the page)"))
		o.Info("  %s", o.Cyan(link))
		o.Info("")
	}
	if !noBrowser && !o.JSON {
		if ui.OpenBrowser(link) {
			o.Info("%s", o.Dim("Opened your browser. Waiting for you to approve…"))
		} else {
			o.Info("%s", o.Dim("Open the link above in a browser. Waiting for you to approve…"))
		}
	}
	sp := o.Spin("Waiting for approval in the browser")
	defer sp.Stop()
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-app.Ctx().Done():
			return fmt.Errorf("cancelled")
		case <-time.After(2 * time.Second):
		}
		var out struct {
			Status string `json:"status"`
			Token  string `json:"token"`
			User   User   `json:"user"`
		}
		if err := app.API.Do(app.Ctx(), "POST", "/api/v1/cli/login/poll", map[string]string{"secret": secret, "name": device}, &out); err != nil {
			if api.IsStatus(err, 429) {
				continue
			}
			return err
		}
		if out.Status == "approved" && out.Token != "" {
			sp.Stop()
			if err := app.saveSignIn(out.Token, &out.User); err != nil {
				return err
			}
			o.Result(map[string]any{"ok": true, "status": "approved", "user": out.User}, func() {
				o.Success("Signed in as %s.", o.Bold(out.User.Email))
			})
			return nil
		}
	}
	return fmt.Errorf("the sign-in link expired. Run `webnak login` again")
}

func loginWithPassword(email, password, code, name string) error {
	o := app.Out
	var err error
	if email == "" {
		if email, err = o.Ask("Email:", app.Cfg.Email); err != nil {
			return err
		}
	}
	password = firstNonEmpty(password, os.Getenv("WEBNAK_PASSWORD"))
	if password == "" {
		if password, err = o.Secret("Password:"); err != nil {
			return err
		}
	}
	body := map[string]string{"email": email, "password": password, "code": code}
	var out struct {
		AccessToken string `json:"accessToken"`
	}
	for attempt := 0; attempt < 4; attempt++ {
		err = app.API.Do(app.Ctx(), "POST", "/api/v1/auth/login", body, &out)
		if err == nil {
			break
		}
		var e *api.Error
		if !asAPIError(err, &e) {
			return err
		}
		if v, _ := e.Data["emailUnverified"].(bool); v {
			return fmt.Errorf("confirm your email first: click the link we sent to %s (or run `webnak resend-verification`)", email)
		}
		mfa, _ := e.Data["mfaRequired"].(bool)
		if !mfa {
			return err
		}
		methods := fmt.Sprint(e.Data["methods"])
		if body["code"] != "" {
			o.Info("%s", o.Red("That code didn't work."))
		}
		if strings.Contains(methods, "email") && !strings.Contains(methods, "totp") && body["code"] == "" {
			mfaToken, _ := e.Data["mfaToken"].(string)
			if err := app.API.Do(app.Ctx(), "POST", "/api/v1/auth/mfa/email/send", map[string]string{"mfaToken": mfaToken}, nil); err == nil {
				o.Info("We emailed a sign-in code to %s.", email)
			}
		}
		label := "Two-factor code (authenticator app, email code or recovery code):"
		if body["code"], err = o.Secret(label); err != nil {
			if strings.Contains(methods, "webauthn") {
				return fmt.Errorf("this account needs a passkey or code. Use `webnak login` (browser) or pass --code")
			}
			return err
		}
	}
	if err != nil {
		return err
	}
	// Trade the short-lived browser-style access token for a CLI token.
	app.API.Token = out.AccessToken
	var tok struct {
		Token string `json:"token"`
		User  User   `json:"user"`
	}
	if err := app.API.Do(app.Ctx(), "POST", "/api/v1/cli/tokens", map[string]string{"name": firstNonEmpty(name, hostName())}, &tok); err != nil {
		return err
	}
	if err := app.saveSignIn(tok.Token, &tok.User); err != nil {
		return err
	}
	o.Result(map[string]any{"ok": true, "user": tok.User}, func() { o.Success("Signed in as %s.", o.Bold(tok.User.Email)) })
	return nil
}

func asAPIError(err error, target **api.Error) bool {
	e, ok := err.(*api.Error)
	if ok {
		*target = e
	}
	return ok
}
