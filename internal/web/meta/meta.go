// Package meta extracts OpenGraph/oEmbed citation metadata and caches widget
// images. Spec §10.8 security guards (https-only, no private IPs, MIME
// allowlist, 10 MB cap, ≤3 redirects, 5 s connect timeout) live in adapter.go.
package meta

import (
	"io"
	"net/url"
	"regexp"
	"strings"
)

type Meta struct {
	Title       string `json:"title,omitempty"`
	Author      string `json:"author,omitempty"`
	SiteName    string `json:"site_name,omitempty"`
	Description string `json:"description,omitempty"`
	Favicon     string `json:"favicon,omitempty"`
}

var (
	reMetaTag    = regexp.MustCompile(`(?is)<meta\s+([^>]+?)/?>`)
	reAttr       = regexp.MustCompile(`(?is)(\w[\w:-]*)\s*=\s*"([^"]*)"`)
	reTitle      = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reLinkIcon   = regexp.MustCompile(`(?is)<link\s+([^>]+?)/?>`)
	reWhitespace = regexp.MustCompile(`\s+`)
)

func ParseMeta(r io.Reader, base string) (Meta, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return Meta{}, err
	}
	s := string(body)
	m := Meta{}
	for _, tag := range reMetaTag.FindAllStringSubmatch(s, -1) {
		attrs := parseAttrs(tag[1])
		prop := strings.ToLower(attrs["property"])
		name := strings.ToLower(attrs["name"])
		content := attrs["content"]
		switch {
		case prop == "og:title" && m.Title == "":
			m.Title = content
		case prop == "og:site_name":
			m.SiteName = content
		case prop == "og:description":
			m.Description = content
		case name == "author":
			m.Author = content
		case name == "description" && m.Description == "":
			m.Description = content
		}
	}
	if m.Title == "" {
		if t := reTitle.FindStringSubmatch(s); len(t) == 2 {
			m.Title = strings.TrimSpace(reWhitespace.ReplaceAllString(t[1], " "))
		}
	}
	for _, link := range reLinkIcon.FindAllStringSubmatch(s, -1) {
		attrs := parseAttrs(link[1])
		rel := strings.ToLower(attrs["rel"])
		if rel == "icon" || rel == "shortcut icon" {
			m.Favicon = resolveURL(base, attrs["href"])
			break
		}
	}
	return m, nil
}

func parseAttrs(s string) map[string]string {
	out := map[string]string{}
	for _, kv := range reAttr.FindAllStringSubmatch(s, -1) {
		out[strings.ToLower(kv[1])] = kv[2]
	}
	return out
}

func resolveURL(base, ref string) string {
	if ref == "" {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(u).String()
}
