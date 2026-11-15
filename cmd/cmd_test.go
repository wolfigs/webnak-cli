package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Harbor Bakery":         "harbor-bakery",
		"  Olive & Ember!  ":    "olive-ember",
		"Café 21":               "caf-21",
		strings.Repeat("a", 60): strings.Repeat("a", 40),
	} {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSiteRefAndStatus(t *testing.T) {
	s := &Site{ID: "abc", Name: "Harbor"}
	if s.Ref() != "abc" || s.Status() != "no address" {
		t.Fatalf("plain site: ref %q status %q", s.Ref(), s.Status())
	}
	s.Hosted = &struct {
		URL    string `json:"url"`
		Live   bool   `json:"live"`
		Source string `json:"source"`
	}{URL: "https://harbor.webnak.site", Live: true}
	if s.Ref() != "harbor" || s.Status() != "live" {
		t.Fatalf("hosted site: ref %q status %q", s.Ref(), s.Status())
	}
	s.TakenDown = true
	if s.Status() != "taken down" {
		t.Fatalf("taken down status %q", s.Status())
	}
}

// The MCP server answers the handshake and lists its tools, each with a schema.
func TestMCPHandshake(t *testing.T) {
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"nope"}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := serveMCP(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d replies, want 3 (none for the notification):\n%s", len(lines), out.String())
	}
	var init struct {
		Result struct {
			ProtocolVersion string                `json:"protocolVersion"`
			ServerInfo      struct{ Name string } `json:"serverInfo"`
		} `json:"result"`
	}
	json.Unmarshal([]byte(lines[0]), &init)
	if init.Result.ProtocolVersion != "2025-06-18" || init.Result.ServerInfo.Name != "webnak" {
		t.Fatalf("initialize = %s", lines[0])
	}
	var list struct {
		Result struct {
			Tools []struct {
				Name        string         `json:"name"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	json.Unmarshal([]byte(lines[1]), &list)
	if len(list.Result.Tools) < 20 {
		t.Fatalf("only %d tools", len(list.Result.Tools))
	}
	for _, tool := range list.Result.Tools {
		if tool.InputSchema["type"] != "object" {
			t.Fatalf("tool %s has no object schema", tool.Name)
		}
	}
	if !strings.Contains(lines[2], `"code":-32601`) {
		t.Fatalf("unknown method reply = %s", lines[2])
	}
}
