// Package api is a small client for the Webnak API.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBase is the production API.
const DefaultBase = "https://api.cloud.wolfigs.com"

// Client calls the API as one signed-in user (or anonymously).
type Client struct {
	Base      string
	Token     string
	UserAgent string
	HTTP      *http.Client
}

// Error is an API error, with the server's own message.
type Error struct {
	Status  int
	Message string
	Data    map[string]any
}

func (e *Error) Error() string { return e.Message }

// IsStatus reports whether err is an API error with this status.
func IsStatus(err error, status int) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == status
}

func New(base, token, version string) *Client {
	if base == "" {
		base = DefaultBase
	}
	return &Client{
		Base:      strings.TrimRight(base, "/"),
		Token:     token,
		UserAgent: "webnak-cli/" + version,
		HTTP:      &http.Client{Timeout: 5 * time.Minute},
	}
}

// Do sends a JSON request and decodes a JSON response into out (if not nil).
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	return c.send(ctx, method, path, rdr, "application/json", out)
}

// Send sends a raw body with a content type, and decodes a JSON response.
func (c *Client) Send(ctx context.Context, method, path string, body io.Reader, contentType string, out any) error {
	return c.send(ctx, method, path, body, contentType, out)
}

// Raw sends a request and returns the response for the caller to read.
func (c *Client) Raw(ctx context.Context, method, path string, extra http.Header) (*http.Response, error) {
	req, err := c.request(ctx, method, path, nil, "")
	if err != nil {
		return nil, err
	}
	for k, v := range extra {
		req.Header[k] = v
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, friendlyNetErr(err)
	}
	if res.StatusCode >= 400 {
		defer res.Body.Close()
		return nil, decodeError(res)
	}
	return res, nil
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader, contentType string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" && body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return req, nil
}

func (c *Client) send(ctx context.Context, method, path string, body io.Reader, contentType string, out any) error {
	req, err := c.request(ctx, method, path, body, contentType)
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return friendlyNetErr(err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return decodeError(res)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("unexpected response from Webnak (%d): %w", res.StatusCode, err)
	}
	return nil
}

func decodeError(res *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	e := &Error{Status: res.StatusCode, Data: map[string]any{}}
	_ = json.Unmarshal(raw, &e.Data)
	if m, ok := e.Data["error"].(string); ok && m != "" {
		e.Message = m
	} else {
		e.Message = strings.TrimSpace(string(raw))
	}
	if e.Message == "" {
		e.Message = http.StatusText(res.StatusCode)
	}
	return e
}

func friendlyNetErr(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) && ue.Timeout() {
		return fmt.Errorf("Webnak took too long to answer. Try again in a moment")
	}
	return fmt.Errorf("could not reach Webnak: %w", err)
}

// Q builds a query string from pairs, skipping empty values.
func Q(kv ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			v.Set(kv[i], kv[i+1])
		}
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

// Path escapes one path segment.
func Path(s string) string { return url.PathEscape(s) }
