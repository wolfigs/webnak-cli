# webnak

The command-line tool for [Webnak](https://webnak.dev): web hosting with a
website editor built in. Put a site online from a template, a folder or a GitHub
repository, at `name.webnak.site` or your own domain, then change anything on it,
from your terminal, your CI, or an AI agent such as Claude Code.

```sh
npx @wolfigs/webnak-cli login
npx @wolfigs/webnak-cli create --template portfolio --name "Ana Lima"
npx @wolfigs/webnak-cli deploy ./dist --create --name "My app"
```

## Install

| | |
|---|---|
| npm (any OS) | `npm i -g @wolfigs/webnak-cli` or run with `npx @wolfigs/webnak-cli` |
| macOS / Linux | `curl -fsSL https://raw.githubusercontent.com/wolfigs/webnak-cli/main/install.sh \| sh` |
| Windows (PowerShell) | `irm https://raw.githubusercontent.com/wolfigs/webnak-cli/main/install.ps1 \| iex` |
| Homebrew | `brew tap wolfigs/webnak https://github.com/wolfigs/webnak-cli && brew install webnak` |
| Binaries | [Releases](https://github.com/wolfigs/webnak-cli/releases) (checksums in `checksums.txt`) |

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
- run: npx -y @wolfigs/webnak-cli deploy ./dist --site harbor --json
  env:
    WEBNAK_TOKEN: ${{ secrets.WEBNAK_TOKEN }}
```

## Claude Code

Add Webnak as an MCP server:

```sh
claude mcp add webnak -- npx -y @wolfigs/webnak-cli@latest mcp
```

Or install the plugin, which adds the MCP server and a skill that teaches Claude
the whole workflow (sign in, templates, deploys, content, domains):

```
/plugin marketplace add wolfigs/webnak-cli
/plugin install webnak@webnak
```

Then ask, for example: *"Build this project and put it on Webnak"* or *"Make me a
portfolio site on Webnak and change the headline to …"*. When Claude needs you to
sign in, it shows a link and a code to approve in your browser; it never sees your
password.

`webnak mcp` works with any MCP client over stdio.

## What the Claude Code plugin runs, sends and stores

The plugin adds one skill (`skills/webnak/SKILL.md`, instructions only) and one
local MCP server. Everything it does goes through the Webnak CLI in this
repository, whose Go source is in `cmd/` and `internal/`.

- **Runs:** `npx -y @wolfigs/webnak-cli@<version> mcp`, pinned to an exact version in
  `.mcp.json`. That npm package is the small wrapper in `npm/bin/webnak.js`. On
  first use it downloads the matching `webnak` binary from this repository's GitHub
  release (`github.com/wolfigs/webnak-cli/releases`), checks it against the
  release's published SHA-256 checksums, caches it, and runs it. Each MCP tool runs
  that binary with `--json --no-input`.
- **Sends:** only to the Webnak API at `https://api.cloud.wolfigs.com`, operated by
  Wolfigs, and only what the user asks for: their account details when signing in,
  the files of a folder they deploy, the text they change, the domains they connect.
  There is no telemetry and no other destination.
- **Opens:** sign-in links on `https://cloud.wolfigs.com`, shown to the user to open
  and approve. The plugin never sees or asks for a password.
- **Stores:** one sign-in token in the user's config directory (see Security below),
  readable only by them, and the downloaded binary in their cache directory.

Webnak's privacy policy: https://webnak.dev/privacy

## Security

- Tokens are stored in your user config directory, readable only by you
  (`~/.config/webnak` on Linux, `~/Library/Application Support/webnak` on macOS,
  `%AppData%\webnak` on Windows).
- Browser sign-in sends only a hash of a secret the CLI keeps; a leaked link can't be
  used to collect a token. Approve sign-ins only when you started them.
- The npm package downloads the binary for your platform from this repository's
  release and checks it against the published SHA-256 checksums. It runs no install
  scripts.

## Releasing

```sh
scripts/bump.sh 0.1.6            # npm package, plugin and the MCP pin, together
git commit -am "0.1.6" && git tag -a v0.1.6 -m v0.1.6 && git push --follow-tags
```

The release workflow builds every platform, publishes the GitHub release, updates
`Formula/webnak.rb` and publishes `@wolfigs/webnak-cli` to npm.

## Build from source

```sh
go build -o webnak .
VERSION=0.1.0 scripts/build.sh   # all platforms into dist/
```

MIT licensed.
