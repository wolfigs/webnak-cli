# webnak

The command-line tool for [Webnak](https://webnak.dev): web hosting with a
website editor built in. Put a site online from a template, a folder or a GitHub
repository, at `name.webnak.site` or your own domain, then change anything on it,
from your terminal, your CI, or an AI agent such as Claude Code.

```sh
npx webnak login
npx webnak create --template portfolio --name "Ana Lima"
npx webnak deploy ./dist --create --name "My app"
```

## Install

| | |
|---|---|
| npm (any OS) | `npm i -g webnak` or run with `npx webnak` |
| macOS / Linux | `curl -fsSL https://raw.githubusercontent.com/sathninduk/webnak-cli/main/install.sh \| sh` |
| Windows (PowerShell) | `irm https://raw.githubusercontent.com/sathninduk/webnak-cli/main/install.ps1 \| iex` |
| Homebrew | `brew tap sathninduk/webnak https://github.com/sathninduk/webnak-cli && brew install webnak` |
| Binaries | [Releases](https://github.com/sathninduk/webnak-cli/releases) (checksums in `checksums.txt`) |

## Sign in

```sh
webnak signup            # create an account (then click the email link)
webnak login             # sign in through your browser (passkeys and 2FA work)
webnak login --password  # or here, with email, password and a 2FA code
webnak whoami
webnak logout
```

For CI and agents, make a token once and set it as `WEBNAK_TOKEN`:

```sh
webnak token create --name github-actions
```

Every sign-in is a session you can see and revoke in Settings → Sessions
(or `webnak account sessions`).

## Common tasks

```sh
webnak templates                                   # 19 templates to start from
webnak create --template restaurant --name "Olive & Ember"
webnak create --dir ./site --name "Harbor Bakery" --subdomain harbor
webnak create --repo you/site --name "My app" --build --command "npm run build" --output dist

webnak sites                                       # your sites
webnak deploy ./dist --site harbor                 # replace the files and go live
webnak deploys harbor                              # history
webnak deploys rollback harbor <deploy-id>
webnak deploys logs harbor                         # why a build failed

webnak edit harbor                                 # open the visual editor in your browser
webnak content set harbor --selector '[data-webnak="hero-title"]' --text "Fresh bread daily"
webnak content publish harbor

webnak files harbor                                # list files
webnak files get harbor index.html
webnak files put harbor about.html ./about.html
webnak files publish harbor                        # put file changes live

webnak domains add harbor harborbakery.com         # prints the DNS records to add
webnak domains check harbor harborbakery.com
webnak download harbor -o harbor.zip               # the site, edits written in

webnak github connect                              # install the GitHub app (browser)
webnak account 2fa totp                            # add an authenticator app
webnak plan                                        # usage; `plan upgrade` opens checkout
```

A site can be named by its subdomain (`harbor`), its address, its name, or its ID.

## Scripts, CI and agents

- `--json` prints machine-readable output on stdout (progress goes to stderr).
- `--no-input` never prompts (on automatically when `CI` is set); missing answers
  fail with a message saying which flag to pass.
- `--yes` confirms destructive actions.
- Exit codes: `0` ok, `1` error, `2` usage, `3` not signed in, `4` not found.
- Environment: `WEBNAK_TOKEN`, `WEBNAK_API`, `WEBNAK_WORKSPACE`, `WEBNAK_NO_BROWSER`,
  `WEBNAK_CONFIG_DIR`.

GitHub Actions:

```yaml
- run: npm run build
- run: npx -y webnak deploy ./dist --site harbor --json
  env:
    WEBNAK_TOKEN: ${{ secrets.WEBNAK_TOKEN }}
```

## Claude Code

Add Webnak as an MCP server:

```sh
claude mcp add webnak -- npx -y webnak mcp
```

Or install the plugin, which adds the MCP server and a skill that teaches Claude
the whole workflow (sign in, templates, deploys, content, domains):

```
/plugin marketplace add sathninduk/webnak-cli
/plugin install webnak@webnak
```

Then ask, for example: *"Build this project and put it on Webnak"* or *"Make me a
portfolio site on Webnak and change the headline to …"*. When Claude needs you to
sign in, it shows a link and a code to approve in your browser; it never sees your
password.

`webnak mcp` works with any MCP client over stdio.

## Security

- Tokens are stored in your user config directory, readable only by you
  (`~/.config/webnak` on Linux, `~/Library/Application Support/webnak` on macOS,
  `%AppData%\webnak` on Windows).
- Browser sign-in sends only a hash of a secret the CLI keeps; a leaked link can't be
  used to collect a token. Approve sign-ins only when you started them.
- The npm package downloads the binary for your platform from this repository's
  release and checks it against the published SHA-256 checksums. It runs no install
  scripts.

## Build from source

```sh
go build -o webnak .
VERSION=0.1.0 scripts/build.sh   # all platforms into dist/
```

MIT licensed.
