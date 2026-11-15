package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wolfigs/webnak-cli/internal/api"
)

var errNotFound = errors.New("not found")

// Site is a site as the API lists it.
type Site struct {
	ID             string   `json:"id"`
	SiteKey        string   `json:"siteKey"`
	Name           string   `json:"name"`
	Origins        []string `json:"origins"`
	Issues         int      `json:"issues"`
	Paused         bool     `json:"paused"`
	Suspended      bool     `json:"suspended"`
	TakenDown      bool     `json:"takenDown"`
	TakedownReason string   `json:"takedownReason,omitempty"`
	Hosted         *struct {
		URL    string `json:"url"`
		Live   bool   `json:"live"`
		Source string `json:"source"`
	} `json:"hosted,omitempty"`
}

// Address is where the site is seen.
func (s *Site) Address() string {
	if s.Hosted != nil && s.Hosted.URL != "" {
		return s.Hosted.URL
	}
	if len(s.Origins) > 0 {
		return s.Origins[0]
	}
	return ""
}

// Ref is how to name the site in a command: its subdomain if hosted, else its ID.
func (s *Site) Ref() string {
	if s.Hosted != nil && s.Hosted.URL != "" {
		h := strings.TrimPrefix(strings.TrimPrefix(s.Hosted.URL, "https://"), "http://")
		return strings.SplitN(h, ".", 2)[0]
	}
	return s.ID
}

// Status is a one-word summary.
func (s *Site) Status() string {
	switch {
	case s.TakenDown:
		return "taken down"
	case s.Suspended:
		return "suspended"
	case s.Paused:
		return "paused"
	case s.Hosted != nil && s.Hosted.Live:
		return "live"
	case s.Hosted != nil:
		return "not live yet"
	case len(s.Origins) == 0:
		return "no address"
	}
	return "installed"
}

func (a *App) sites() ([]Site, error) {
	var out []Site
	err := a.API.Do(a.Ctx(), "GET", "/api/v1/webnak/sites"+api.Q("workspaceId", a.Workspace), nil, &out)
	return out, err
}

// findSite resolves a site by ID, subdomain, address or (unique) name.
func (a *App) findSite(ref string) (*Site, error) {
	if err := a.needAuth(); err != nil {
		return nil, err
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("which site? Give its name, address or ID (see `webnak sites`)")
	}
	all, err := a.sites()
	if err != nil {
		return nil, err
	}
	low := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(ref, "https://"), "http://"), "/"))
	var byName []*Site
	for i := range all {
		s := &all[i]
		addr := strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(s.Address(), "https://"), "http://"))
		sub := strings.SplitN(addr, ".", 2)[0]
		if s.ID == ref || s.SiteKey == ref || addr == low || (s.Hosted != nil && sub == low) {
			return s, nil
		}
		if strings.EqualFold(s.Name, ref) {
			byName = append(byName, s)
		}
	}
	if len(byName) == 1 {
		return byName[0], nil
	}
	if len(byName) > 1 {
		return nil, fmt.Errorf("%d sites are called %q; use the ID instead (see `webnak sites`)", len(byName), ref)
	}
	return nil, fmt.Errorf("%w: no site matches %q (see `webnak sites`)", errNotFound, ref)
}

// pickSite resolves ref, or asks which site when it is empty.
func (a *App) pickSite(ref string) (*Site, error) {
	if ref != "" {
		return a.findSite(ref)
	}
	if err := a.needAuth(); err != nil {
		return nil, err
	}
	all, err := a.sites()
	if err != nil {
		return nil, err
	}
	if len(all) == 1 {
		return &all[0], nil
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("you have no sites yet. Make one with `webnak create`")
	}
	names := make([]string, len(all))
	for i, s := range all {
		names[i] = fmt.Sprintf("%s  %s", s.Name, a.Out.Dim(s.Address()))
	}
	i, err := a.Out.Choose("Which site?", names)
	if err != nil {
		return nil, fmt.Errorf("which site? Pass --site (see `webnak sites`)")
	}
	return &all[i], nil
}
