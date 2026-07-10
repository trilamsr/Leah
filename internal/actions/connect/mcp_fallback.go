package connect

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

// mcpFallbackProvider is the connect Provider for integrations that ship only
// an MCP server (no OAuth, no static token). The "credential" is the MCP
// endpoint URL — the adapter's caller dials it via the project's MCP host.
//
// Persisting via the same oauth2.Token + 0600 token file (AccessToken := URL)
// keeps the Authorize → WriteToken → DefaultRegistry.List wiring identical to
// every other provider; downstream readers don't branch on adapter kind.
type mcpFallbackProvider struct {
	name      string
	tokenPath string
	in        io.Reader
	out       io.Writer
}

func newMCPFallback(name string, in io.Reader, out io.Writer) *mcpFallbackProvider {
	return &mcpFallbackProvider{
		name:      name,
		tokenPath: DefaultTokenPath(name),
		in:        in,
		out:       out,
	}
}

func (p *mcpFallbackProvider) Name() string      { return p.name }
func (p *mcpFallbackProvider) Scopes() []string  { return []string{"connect:" + p.name} }
func (p *mcpFallbackProvider) TokenPath() string { return p.tokenPath }

func (p *mcpFallbackProvider) authorize(_ context.Context, _ PromptFn) (*oauth2.Token, error) {
	r := bufio.NewReader(p.in)
	_, _ = fmt.Fprintf(p.out, "MCP endpoint URL for %s (e.g. https://mcp.example.com): ", p.name)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return nil, fmt.Errorf("connect %s: read endpoint: %w", p.name, err)
	}
	v := strings.TrimSpace(line)
	if v == "" {
		return nil, fmt.Errorf("connect %s: empty endpoint", p.name)
	}
	// Reject bare hosts / shell strings up-front — a malformed URL would only
	// surface at first call against the adapter, long after consent landed.
	u, perr := url.Parse(v)
	if perr != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("connect %s: invalid endpoint %q", p.name, v)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("connect %s: endpoint scheme must be http/https, got %q", p.name, u.Scheme)
	}
	return &oauth2.Token{AccessToken: v, TokenType: "mcp"}, nil
}
