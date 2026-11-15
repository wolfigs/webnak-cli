package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// webnak mcp: a Model Context Protocol server over stdio, so AI agents such as
// Claude Code can use Webnak as tools:
//
//	claude mcp add webnak -- npx -y @wolfigs/webnak-cli mcp
//
// Each tool runs this same CLI (with --json --no-input) and returns what it
// prints, so the tools and the commands can never disagree.

type mcpTool struct {
	Name        string                                            `json:"name"`
	Description string                                            `json:"description"`
	InputSchema map[string]any                                    `json:"inputSchema"`
	args        func(in map[string]any) ([]string, string, error) // argv, stdin
}

func prop(t, desc string) map[string]any { return map[string]any{"type": t, "description": desc} }

func schema(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(in map[string]any, k string) string {
	if v, ok := in[k].(string); ok {
		return v
	}
	return ""
}

func flag(argv []string, name, v string) []string {
	if v != "" {
		return append(argv, "--"+name, v)
	}
	return argv
}

var siteProp = prop("string", "The site: its name, address (e.g. harbor.webnak.site) or ID")

var mcpTools = []mcpTool{
	{Name: "whoami", Description: "Show the signed-in Webnak account. Fails with exit code 3 if not signed in.", InputSchema: schema(nil, map[string]any{}),
		args: func(in map[string]any) ([]string, string, error) { return []string{"whoami"}, "", nil }},
	{Name: "login", Description: "Sign in to Webnak in the browser. Returns a link and code for the user to open and approve, then waits (up to 10 minutes) until they do. Tell the user to open the link.",
		InputSchema: schema(nil, map[string]any{}),
		args:        func(in map[string]any) ([]string, string, error) { return []string{"login", "--no-browser"}, "", nil }},
	{Name: "list_sites", Description: "List the user's Webnak sites with address, status and ID.", InputSchema: schema(nil, map[string]any{}),
		args: func(in map[string]any) ([]string, string, error) { return []string{"sites"}, "", nil }},
	{Name: "site_info", Description: "Show one site: address, status, source and latest deploy.", InputSchema: schema([]string{"site"}, map[string]any{"site": siteProp}),
		args: func(in map[string]any) ([]string, string, error) {
			return []string{"sites", "info", str(in, "site")}, "", nil
		}},
	{Name: "list_templates", Description: "List the website templates a site can start from (IDs for create_site).", InputSchema: schema(nil, map[string]any{}),
		args: func(in map[string]any) ([]string, string, error) { return []string{"templates"}, "", nil }},
	{Name: "create_site", Description: "Create a site hosted by Webnak at <subdomain>.webnak.site from a template, a local folder (with index.html) or a connected GitHub repository, and wait until it is live. Give exactly one of template, dir or repo.",
		InputSchema: schema([]string{"name"}, map[string]any{
			"name": prop("string", "Site name"), "subdomain": prop("string", "Address: <subdomain>.webnak.site (default from the name)"),
			"template": prop("string", "Template ID from list_templates"), "dir": prop("string", "Local folder to upload (absolute path)"),
			"repo": prop("string", "GitHub owner/name (GitHub must be connected)"), "build_command": prop("string", "Build command for a repo, e.g. npm run build"),
			"output": prop("string", "Build output folder, e.g. dist"),
		}),
		args: func(in map[string]any) ([]string, string, error) {
			a := []string{"create", "--name", str(in, "name")}
			a = flag(a, "subdomain", str(in, "subdomain"))
			a = flag(a, "template", str(in, "template"))
			a = flag(a, "dir", str(in, "dir"))
			a = flag(a, "repo", str(in, "repo"))
			a = flag(a, "command", str(in, "build_command"))
			a = flag(a, "output", str(in, "output"))
			return a, "", nil
		}},
	{Name: "deploy", Description: "Upload a local folder (must contain index.html; for a build, its output such as dist) to a hosted site, replacing its files, and wait until live. With create=true and no matching site, a new site is created. For a GitHub site, omit dir to rebuild the latest commit.",
		InputSchema: schema(nil, map[string]any{"dir": prop("string", "Absolute path of the folder to deploy"), "site": siteProp,
			"create": prop("boolean", "Create the site if it doesn't exist"), "name": prop("string", "Name for a new site"), "subdomain": prop("string", "Subdomain for a new site")}),
		args: func(in map[string]any) ([]string, string, error) {
			a := []string{"deploy"}
			if d := str(in, "dir"); d != "" {
				a = append(a, d)
			}
			a = flag(a, "site", str(in, "site"))
			if in["create"] == true {
				a = append(a, "--create")
			}
			a = flag(a, "name", str(in, "name"))
			a = flag(a, "subdomain", str(in, "subdomain"))
			return a, "", nil
		}},
	{Name: "list_deploys", Description: "List a hosted site's deploys.", InputSchema: schema([]string{"site"}, map[string]any{"site": siteProp}),
		args: func(in map[string]any) ([]string, string, error) {
			return []string{"deploys", str(in, "site")}, "", nil
		}},
	{Name: "deploy_logs", Description: "Show a deploy's build log (latest by default), to see why a build failed.", InputSchema: schema([]string{"site"}, map[string]any{"site": siteProp, "deploy_id": prop("string", "Deploy ID (optional)")}),
		args: func(in map[string]any) ([]string, string, error) {
			a := []string{"deploys", "logs", str(in, "site")}
			if d := str(in, "deploy_id"); d != "" {
				a = append(a, d)
			}
			return a, "", nil
		}},
	{Name: "rollback", Description: "Put an earlier deploy of a hosted site back live.", InputSchema: schema([]string{"site", "deploy_id"}, map[string]any{"site": siteProp, "deploy_id": prop("string", "Deploy ID from list_deploys")}),
		args: func(in map[string]any) ([]string, string, error) {
			return []string{"deploys", "rollback", str(in, "site"), str(in, "deploy_id")}, "", nil
		}},
	{Name: "list_files", Description: "List a hosted site's files and any changes not yet live.", InputSchema: schema([]string{"site"}, map[string]any{"site": siteProp}),
		args: func(in map[string]any) ([]string, string, error) { return []string{"files", str(in, "site")}, "", nil }},
	{Name: "read_file", Description: "Read one file of a hosted site (as it will be after pending changes).", InputSchema: schema([]string{"site", "path"}, map[string]any{"site": siteProp, "path": prop("string", "File path, e.g. index.html")}),
		args: func(in map[string]any) ([]string, string, error) {
			return []string{"files", "get", str(in, "site"), str(in, "path")}, "", nil
		}},
	{Name: "write_file", Description: "Write one file of a hosted site (a pending change; call publish_files to put changes live).", InputSchema: schema([]string{"site", "path", "content"}, map[string]any{"site": siteProp, "path": prop("string", "File path"), "content": prop("string", "Full new file content")}),
		args: func(in map[string]any) ([]string, string, error) {
			return []string{"files", "put", str(in, "site"), str(in, "path"), "-"}, str(in, "content"), nil
		}},
	{Name: "delete_files", Description: "Delete files of a hosted site (a folder with a trailing /), as pending changes.", InputSchema: schema([]string{"site", "paths"}, map[string]any{"site": siteProp, "paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}),
		args: func(in map[string]any) ([]string, string, error) {
			a := []string{"files", "rm", str(in, "site")}
			if l, ok := in["paths"].([]any); ok {
				for _, p := range l {
					a = append(a, fmt.Sprint(p))
				}
			}
			return a, "", nil
		}},
	{Name: "publish_files", Description: "Put a hosted site's pending file changes live and wait.", InputSchema: schema([]string{"site"}, map[string]any{"site": siteProp}),
		args: func(in map[string]any) ([]string, string, error) {
			return []string{"files", "publish", str(in, "site")}, "", nil
		}},
	{Name: "set_content", Description: "Change an element's text, HTML or an attribute on a page as a draft (the visual editor's way, no files touched). Template elements have data-webnak names, e.g. selector [data-webnak=\"hero-title\"]. Call publish_content after.",
		InputSchema: schema([]string{"site", "selector"}, map[string]any{"site": siteProp, "selector": prop("string", "CSS selector of the element"),
			"page": prop("string", "Page path, default /"), "text": prop("string", "New text"), "html": prop("string", "New inner HTML"), "attr": prop("string", "Attribute as name=value")}),
		args: func(in map[string]any) ([]string, string, error) {
			a := []string{"content", "set", str(in, "site"), "--selector", str(in, "selector")}
			a = flag(a, "page", str(in, "page"))
			if _, ok := in["text"]; ok {
				a = append(a, "--text", str(in, "text"))
			}
			if _, ok := in["html"]; ok {
				a = append(a, "--html", str(in, "html"))
			}
			a = flag(a, "attr", str(in, "attr"))
			return a, "", nil
		}},
	{Name: "publish_content", Description: "Put a page's draft content edits live.", InputSchema: schema([]string{"site"}, map[string]any{"site": siteProp, "page": prop("string", "Page path, default /")}),
		args: func(in map[string]any) ([]string, string, error) {
			return flag([]string{"content", "publish", str(in, "site")}, "page", str(in, "page")), "", nil
		}},
	{Name: "edit_link", Description: "Get a private link that opens the visual editor on the live site, for the user to edit by clicking.", InputSchema: schema([]string{"site"}, map[string]any{"site": siteProp, "page": prop("string", "Page path")}),
		args: func(in map[string]any) ([]string, string, error) {
			return flag([]string{"edit", str(in, "site")}, "page", str(in, "page")), "", nil
		}},
	{Name: "list_domains", Description: "List a hosted site's custom domains.", InputSchema: schema([]string{"site"}, map[string]any{"site": siteProp}),
		args: func(in map[string]any) ([]string, string, error) {
			return []string{"domains", str(in, "site")}, "", nil
		}},
	{Name: "add_domain", Description: "Connect a custom domain (with its www/bare pair) to a hosted site; returns the DNS records the user must add at their domain provider. Needs the Starter plan.",
		InputSchema: schema([]string{"site", "domain"}, map[string]any{"site": siteProp, "domain": prop("string", "e.g. harborbakery.com")}),
		args: func(in map[string]any) ([]string, string, error) {
			return []string{"domains", "add", str(in, "site"), str(in, "domain")}, "", nil
		}},
	{Name: "check_domain", Description: "Check a custom domain's DNS and HTTPS status.", InputSchema: schema([]string{"site", "domain"}, map[string]any{"site": siteProp, "domain": prop("string", "Domain")}),
		args: func(in map[string]any) ([]string, string, error) {
			return []string{"domains", "check", str(in, "site"), str(in, "domain")}, "", nil
		}},
	{Name: "github_repos", Description: "List GitHub repositories Webnak can host (after the user connects GitHub with `webnak github connect`).", InputSchema: schema(nil, map[string]any{}),
		args: func(in map[string]any) ([]string, string, error) { return []string{"github", "repos"}, "", nil }},
	{Name: "webnak", Description: "Run any webnak CLI command not covered by the other tools (for example: [\"plan\"], [\"workspaces\"], [\"download\",\"my-site\",\"-o\",\"/tmp/site.zip\"], [\"sites\",\"delete\",\"my-site\",\"--yes\"]). Pass the arguments after `webnak`. Output is JSON. Run [\"--help\"] or [\"<command>\",\"--help\"] to see what's available.",
		InputSchema: schema([]string{"args"}, map[string]any{"args": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Arguments after `webnak`"}}),
		args: func(in map[string]any) ([]string, string, error) {
			var a []string
			if l, ok := in["args"].([]any); ok {
				for _, x := range l {
					a = append(a, fmt.Sprint(x))
				}
			}
			if len(a) > 0 && a[0] == "mcp" {
				return nil, "", fmt.Errorf("can't run the MCP server from inside itself")
			}
			return a, "", nil
		}},
}

func init() {
	root.AddCommand(&cobra.Command{
		Use:   "mcp",
		Short: "Run as an MCP server for AI agents (Claude Code: claude mcp add webnak -- npx -y @wolfigs/webnak-cli mcp)",
		Long: `Serve Webnak's actions as Model Context Protocol tools over stdio.

Claude Code:   claude mcp add webnak -- npx -y @wolfigs/webnak-cli mcp
Other clients: run "webnak mcp" as a stdio server.

Sign in first with ` + "`webnak login`" + ` (or set WEBNAK_TOKEN in the server's environment).`,
		RunE: func(cmd *cobra.Command, args []string) error { return serveMCP(cmd.Context(), os.Stdin, os.Stdout) },
	})
}

func serveMCP(ctx context.Context, in io.Reader, out io.Writer) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	r := bufio.NewReaderSize(in, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var msg struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if json.Unmarshal(line, &msg) != nil {
				_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": nil, "error": map[string]any{"code": -32700, "message": "parse error"}})
			} else if len(msg.ID) > 0 { // notifications (no id) need no answer
				res, rpcErr := handleMCP(ctx, self, msg.Method, msg.Params)
				reply := map[string]any{"jsonrpc": "2.0", "id": msg.ID}
				if rpcErr != nil {
					reply["error"] = rpcErr
				} else {
					reply["result"] = res
				}
				_ = enc.Encode(reply)
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func handleMCP(ctx context.Context, self, method string, params json.RawMessage) (any, map[string]any) {
	switch method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(params, &p)
		v := p.ProtocolVersion
		if v == "" {
			v = "2025-06-18"
		}
		return map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "webnak", "version": Version},
			"instructions": "Webnak hosts websites (at <name>.webnak.site or a custom domain) and lets people edit them by clicking. " +
				"Typical flow: whoami (if not signed in, call login and give the user the link), list_templates, create_site or deploy a built folder, " +
				"then share the URL; use set_content + publish_content for text changes, or edit_link for the user to edit visually. " +
				"Destructive actions (deleting sites) need explicit user confirmation.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools}, nil
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, map[string]any{"code": -32602, "message": "invalid params"}
		}
		for _, t := range mcpTools {
			if t.Name != p.Name {
				continue
			}
			if p.Arguments == nil {
				p.Arguments = map[string]any{}
			}
			argv, stdin, err := t.args(p.Arguments)
			if err != nil {
				return toolResult(err.Error(), true), nil
			}
			text, failed := runSelf(ctx, self, argv, stdin, t.Name == "login")
			return toolResult(text, failed), nil
		}
		return nil, map[string]any{"code": -32602, "message": "unknown tool " + p.Name}
	}
	return nil, map[string]any{"code": -32601, "message": "method not found: " + method}
}

func toolResult(text string, isErr bool) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isErr}
}

// runSelf runs this CLI with argv and returns its JSON output (or error).
func runSelf(ctx context.Context, self string, argv []string, stdin string, long bool) (string, bool) {
	timeout := 30 * time.Minute
	if long {
		timeout = 11 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c := exec.CommandContext(ctx, self, append([]string{"--json", "--no-input"}, argv...)...)
	c.Env = append(os.Environ(), "WEBNAK_NO_BROWSER=1")
	if stdin != "" {
		c.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	err := c.Run()
	text := strings.TrimSpace(stdout.String())
	if text == "" {
		text = strings.TrimSpace(stderr.String())
	}
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, true
	}
	if text == "" {
		text = `{"ok":true}`
	}
	return text, false
}
