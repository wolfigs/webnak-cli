---
name: webnak
description: Put a website online and edit it with Webnak (web hosting with a website editor built in). Use when the user wants to host, publish, deploy or share a website, start one from a template, deploy a built folder (dist, out, build) or a GitHub repository, change text or images on a live site, connect a custom domain, roll back a deploy, or download their site. Works through the `webnak` CLI or the Webnak MCP tools.
---

# Webnak

Webnak hosts websites at `<name>.webnak.site` (or the user's own domain) and lets
anyone edit them by clicking. Use the Webnak MCP tools when they are available;
otherwise run the `webnak` CLI (install: `npm i -g @wolfigs/webnak-cli`, or run `npx -y @wolfigs/webnak-cli`).
Always pass `--json --no-input` to the CLI so it never waits on a prompt, and read
its JSON output. Exit codes: 0 ok, 1 error, 2 usage, 3 not signed in, 4 not found.

## 1. Sign in

- Check first: `webnak whoami --json` (MCP: `whoami`).
- Not signed in (exit code 3): run `webnak login --json --no-browser` (MCP: `login`).
  It prints a link and a short code, then waits up to 10 minutes. **Show the user
  the link and code and ask them to open it and approve.** Never approve it yourself
  and never ask for their password.
- In CI, use a token instead: `WEBNAK_TOKEN=wnk_…` (made with `webnak token create`).
- No account yet: the user can run `webnak signup` themselves, or sign up in the
  browser; they must click the confirmation email before signing in.

## 2. Put a site online

- **From a template:** `webnak templates --json`, pick one that fits, then
  `webnak create --template <id> --name "<Site name>" --json --no-input`.
- **From a folder the user (or you) built:** build first (e.g. `npm run build`), then
  deploy the folder that contains `index.html` (often `dist`, `out` or `build`):
  `webnak deploy ./dist --create --name "<Site name>" --json --no-input`.
  To update an existing site: `webnak deploy ./dist --site <site> --json --no-input`.
  For Next.js, the site must be a static export (`output: "export"`).
- **From GitHub:** the user connects GitHub once with `webnak github connect`
  (in their browser). Then `webnak create --repo owner/name --name "…" --build
  --json --no-input`; Webnak detects the framework and builds it. Later,
  `webnak deploy --site <site>` rebuilds the latest commit.

Commands wait until the site is live and print its URL. Share the URL with the user.
If a build fails, read `webnak deploys logs <site> --json` and fix the cause.

The free plan hosts one site; more sites and custom domains need Starter
(`webnak plan upgrade` opens checkout in the browser; payment is the user's to do).

## 3. Change content

- Let the user edit visually: `webnak edit <site> --json` returns a private link that
  opens the editor on the live page. Give it only to the user.
- Change text yourself: templates mark editable elements with `data-webnak`
  (see `webnak files get <site> index.html`). Then
  `webnak content set <site> --selector '[data-webnak="hero-title"]' --text "…" --json`
  and `webnak content publish <site> --json`.
- Change files: `webnak files get|put|rm|mv <site> …`, then `webnak files publish <site>`
  (puts all pending file changes live in one deploy). `webnak files discard` undoes them.

## 4. Domains, deploys, everything else

- Custom domain: `webnak domains add <site> example.com --json` returns DNS records;
  tell the user to add them at their domain provider, then
  `webnak domains check <site> example.com --json`.
- Deploy history and rollback: `webnak deploys <site>`, `webnak deploys rollback <site> <id>`.
- Download the site with edits written in: `webnak download <site> -o site.zip`.
- Everything else: `webnak --help` and `webnak <command> --help`.

## Care

- Deleting a site (`webnak sites delete <site> --yes`) cannot be undone: confirm
  with the user first, in their words.
- Don't put tokens, passwords or edit links in files that will be committed or deployed.
