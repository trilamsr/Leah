package mcp

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/selflearn"
)

const maxSearchRows = 200

// Tools holds the read-only tool surface (S11 W138). Hosts the four
// /tools/* handlers registered onto a *Server.
type Tools struct {
	Server    *Server
	MemoryDir string
	AuditPath string
	Now       func() time.Time
}

// Register wires the four read tools onto the server's mux.
func (t *Tools) Register() {
	t.Server.Register("leah_get_memory_rule", t.toolGetMemoryRule)
	t.Server.Register("leah_search_audit", t.toolSearchAudit)
	t.Server.Register("leah_dispatch_status", t.toolDispatchStatus)
	t.Server.Register("leah_self_build_status", t.toolSelfBuildStatus)
}

type memoryRuleOut struct {
	Name         string `json:"name"`
	BodyMD       string `json:"body_md"`
	SHA256       string `json:"sha256"`
	MtimeRFC3339 string `json:"mtime_rfc3339"`
}

func (t *Tools) toolGetMemoryRule(body []byte) (any, int, error) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, http.StatusBadRequest, errors.New("invalid_json")
	}
	// Strip trailing .md so name="rule" and name="rule.md" both resolve.
	name := strings.TrimSuffix(in.Name, ".md")
	clean := filepath.Clean(name)
	if clean != name || clean == "" || clean == "." || strings.ContainsAny(clean, "/\\") || strings.HasPrefix(clean, ".") {
		return nil, http.StatusBadRequest, errors.New("invalid_name")
	}
	path := filepath.Join(t.MemoryDir, clean+".md")
	// Stat BEFORE ReadFile: a race that removes the file converts to a clean
	// ENOENT on ReadFile rather than panicking on a nil FileInfo.
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, http.StatusNotFound, errors.New("unknown_rule")
		}
		return nil, http.StatusInternalServerError, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, http.StatusNotFound, errors.New("unknown_rule")
		}
		return nil, http.StatusInternalServerError, err
	}
	sum := sha256.Sum256(raw)
	return memoryRuleOut{
		Name:         clean,
		BodyMD:       string(raw),
		SHA256:       hex.EncodeToString(sum[:]),
		MtimeRFC3339: info.ModTime().UTC().Format(time.RFC3339),
	}, 200, nil
}

type searchAuditOut struct {
	Rows      []audit.Entry `json:"rows"`
	Truncated bool          `json:"truncated"`
}

func (t *Tools) toolSearchAudit(body []byte) (any, int, error) {
	var in struct {
		Query string `json:"query"`
		Since string `json:"since"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return nil, http.StatusBadRequest, errors.New("invalid_json")
		}
	}
	if in.Since != "" {
		if _, err := time.Parse(time.RFC3339, in.Since); err != nil {
			t.Server.audit("mcp_request_invalid", "tool=leah_search_audit field=since", 0, "rejected")
			return nil, http.StatusBadRequest, errors.New("invalid_since")
		}
	}
	out, err := t.searchAudit(in.Query, in.Since)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return out, 200, nil
}

func (t *Tools) searchAudit(query, since string) (searchAuditOut, error) {
	entries, err := readEntries(t.AuditPath)
	if err != nil {
		return searchAuditOut{}, err
	}
	var sinceTS time.Time
	if since != "" {
		sinceTS, _ = time.Parse(time.RFC3339, since)
	}
	q := strings.ToLower(query)
	matched := make([]audit.Entry, 0, len(entries))
	for _, e := range entries {
		if !sinceTS.IsZero() {
			ts, err := time.Parse(time.RFC3339, e.Timestamp)
			if err != nil || ts.Before(sinceTS) {
				continue
			}
		}
		if q != "" && !rowMatches(e, q) {
			continue
		}
		if hit, _ := RedactHit(e.Detail); hit {
			t.Server.audit("mcp_redact_drop", "args_hash="+argsHash(e), 0, "dropped")
			continue
		}
		if hit, _ := RedactHit(e.Workspace); hit {
			t.Server.audit("mcp_redact_drop", "args_hash="+argsHash(e), 0, "dropped")
			continue
		}
		matched = append(matched, e)
	}
	truncated := false
	if len(matched) > maxSearchRows {
		matched = matched[len(matched)-maxSearchRows:]
		truncated = true
	}
	return searchAuditOut{Rows: matched, Truncated: truncated}, nil
}

func rowMatches(e audit.Entry, q string) bool {
	return strings.Contains(strings.ToLower(e.Kind), q) ||
		strings.Contains(strings.ToLower(e.Outcome), q) ||
		strings.Contains(strings.ToLower(e.Detail), q) ||
		strings.Contains(strings.ToLower(e.Workspace), q)
}

func argsHash(e audit.Entry) string {
	if e.ArgsHash != "" && len(e.ArgsHash) >= 8 {
		return e.ArgsHash[:8]
	}
	sum := sha256.Sum256([]byte(e.Timestamp + e.Kind + e.Detail))
	return hex.EncodeToString(sum[:4])
}

type dispatchInFlight struct {
	Kind      string `json:"kind"`
	ArgsHash  string `json:"args_hash"`
	StartedTS string `json:"started_ts"`
	BR        int    `json:"br"`
}

type dispatchStatusOut struct {
	InFlight []dispatchInFlight `json:"in_flight"`
	Recent   []audit.Entry      `json:"recent_completed"`
}

func (t *Tools) toolDispatchStatus(_ []byte) (any, int, error) {
	out, err := t.dispatchStatus()
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return out, 200, nil
}

// dispatchStatus filters audit to dispatcher kinds; in-flight = dispatched
// without matching outcome row. Scope: only `self-build` emits
// outcome="dispatched", so InFlight is effectively self-build-only today.
// `ship` and `ask` rows show up in Recent but never InFlight.
func (t *Tools) dispatchStatus() (dispatchStatusOut, error) {
	entries, err := readEntries(t.AuditPath)
	if err != nil {
		return dispatchStatusOut{}, err
	}
	dispatcherKinds := map[string]bool{"ship": true, "self-build": true, "ask": true}
	recent := make([]audit.Entry, 0)
	inflight := make(map[string]dispatchInFlight)
	for _, e := range entries {
		if !dispatcherKinds[e.Kind] {
			continue
		}
		recent = append(recent, e)
		key := e.Kind + ":" + e.ArgsHash
		if e.Outcome == "dispatched" {
			inflight[key] = dispatchInFlight{
				Kind: e.Kind, ArgsHash: e.ArgsHash, StartedTS: e.Timestamp, BR: e.BlastRadius,
			}
		} else if e.Outcome != "" {
			delete(inflight, key)
		}
	}
	flat := make([]dispatchInFlight, 0, len(inflight))
	for _, v := range inflight {
		flat = append(flat, v)
	}
	sort.Slice(flat, func(i, j int) bool { return flat[i].StartedTS < flat[j].StartedTS })
	if len(recent) > 50 {
		recent = recent[len(recent)-50:]
	}
	return dispatchStatusOut{InFlight: flat, Recent: recent}, nil
}

type selfBuildStatusOut struct {
	Dangling []selflearn.DanglingSelfBuild `json:"dangling"`
}

func (t *Tools) toolSelfBuildStatus(_ []byte) (any, int, error) {
	out, err := t.selfBuildStatus()
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return out, 200, nil
}

func (t *Tools) selfBuildStatus() (selfBuildStatusOut, error) {
	now := t.Now
	if now == nil {
		now = time.Now
	}
	d, err := selflearn.DetectDanglingSelfBuild(t.AuditPath, now)
	if err != nil {
		return selfBuildStatusOut{}, err
	}
	return selfBuildStatusOut{Dangling: d}, nil
}

func readEntries(path string) ([]audit.Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open audit: %w", err)
	}
	defer func() { _ = f.Close() }()
	var out []audit.Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e audit.Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, sc.Err()
}
