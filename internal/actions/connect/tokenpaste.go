package connect

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"golang.org/x/oauth2"
)

// tokenPasteProvider is the connect Provider for static-credential (no-OAuth) work tools.
type tokenPasteProvider struct {
	name      string
	tokenPath string
	fields    []string
	assemble  func([]string) string
	in        io.Reader
	out       io.Writer
}

type tokenPasteSpec struct {
	name     string
	fields   []string
	assemble func([]string) string
}

func newTokenPaste(spec tokenPasteSpec, in io.Reader, out io.Writer) *tokenPasteProvider {
	return &tokenPasteProvider{
		name:      spec.name,
		tokenPath: DefaultTokenPath(spec.name),
		fields:    spec.fields,
		assemble:  spec.assemble,
		in:        in,
		out:       out,
	}
}

func (p *tokenPasteProvider) Name() string      { return p.name }
func (p *tokenPasteProvider) Scopes() []string  { return []string{"connect:" + p.name} }
func (p *tokenPasteProvider) TokenPath() string { return p.tokenPath }

func (p *tokenPasteProvider) authorize(_ context.Context, _ PromptFn) (*oauth2.Token, error) {
	r := bufio.NewReader(p.in)
	vals := make([]string, len(p.fields))
	for i, f := range p.fields {
		_, _ = fmt.Fprintf(p.out, "%s: ", f)
		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			return nil, fmt.Errorf("connect %s: read %s: %w", p.name, f, err)
		}
		v := strings.TrimSpace(line)
		if v == "" {
			return nil, fmt.Errorf("connect %s: empty %s", p.name, f)
		}
		vals[i] = v
	}
	return &oauth2.Token{AccessToken: p.assemble(vals)}, nil
}

// rawToken stores the pasted credential verbatim; the adapter's transport handles any prefix.
func rawToken(v []string) string { return v[0] }
