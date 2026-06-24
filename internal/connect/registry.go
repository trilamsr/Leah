package connect

import (
	"os"
	"sort"
)

type Registry struct {
	providers []Provider
	byName    map[string]Provider
}

func NewRegistry(providers []Provider) *Registry {
	byName := make(map[string]Provider, len(providers))
	for _, p := range providers {
		byName[p.Name()] = p
	}
	return &Registry{providers: providers, byName: byName}
}

func DefaultRegistry() *Registry {
	return NewRegistry([]Provider{
		NewGmail(os.Getenv("LEAH_GMAIL_CLIENT_ID"), os.Getenv("LEAH_GMAIL_CLIENT_SECRET")),
		NewGcal(os.Getenv("LEAH_GCAL_CLIENT_ID"), os.Getenv("LEAH_GCAL_CLIENT_SECRET")),
		NewGitHub(os.Getenv("LEAH_GITHUB_CLIENT_ID"), os.Getenv("LEAH_GITHUB_CLIENT_SECRET")),
		NewRegatta(),
		newTokenPaste(tokenPasteSpec{name: "confluence", fields: []string{"Atlassian email", "API token"}, assemble: basicAuth}, os.Stdin, os.Stdout),
		newTokenPaste(tokenPasteSpec{name: "jira", fields: []string{"Atlassian email", "API token"}, assemble: basicAuth}, os.Stdin, os.Stdout),
		newTokenPaste(tokenPasteSpec{name: "slack", fields: []string{"Bot token (xoxb-…)"}, assemble: rawToken}, os.Stdin, os.Stdout),
		newTokenPaste(tokenPasteSpec{name: "notion", fields: []string{"Integration secret"}, assemble: rawToken}, os.Stdin, os.Stdout),
		newTokenPaste(tokenPasteSpec{name: "linear", fields: []string{"API key (lin_api_…)"}, assemble: rawToken}, os.Stdin, os.Stdout),
		newTokenPaste(tokenPasteSpec{name: "msteams", fields: []string{"Graph API bearer token"}, assemble: rawToken}, os.Stdin, os.Stdout),
		newTokenPaste(tokenPasteSpec{name: "tmdb", fields: []string{"TMDB v4 read token (eyJ...)"}, assemble: rawToken}, os.Stdin, os.Stdout),
		// Discord has no RFC-8628 device flow; Bot Tokens are pasted by the
		// operator after creating an Application at discord.com/developers.
		newTokenPaste(tokenPasteSpec{name: "discord", fields: []string{"Bot token"}, assemble: rawToken}, os.Stdin, os.Stdout),
		newNativeMac("imessage", "osascript"),
		newNativeMac("facetime", "open"),
	})
}

func (r *Registry) Lookup(name string) (Provider, error) {
	if p, ok := r.byName[name]; ok {
		return p, nil
	}
	return nil, ErrUnknownProvider
}

// List enumerates providers; zero-byte token files are treated as missing to
// short-circuit a half-written-token deadlock on adapter deserialize.
func (r *Registry) List() []Status {
	out := make([]Status, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, Status{
			Name:       p.Name(),
			Authorized: tokenAuthorized(p.TokenPath()),
			TokenPath:  p.TokenPath(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func tokenAuthorized(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	return st.Size() > 0
}
