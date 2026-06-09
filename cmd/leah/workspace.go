package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/persona"
)

// runWorkspace dispatches `leah workspace <action> ...`.
//
// Workspace is the operator-facing noun for the dimension implemented in
// internal/ctxmgr (where the persisted entity is named "context"; mapping
// rule context.name == workspace_id per docs/specs/2026-06-09-context-manager.md
// §2.1). The list/new/switch/show subcommands are thin aliases that delegate
// to runCtx so behavior stays single-sourced; persona set/show is new and
// owned here.
func runWorkspace(args []string) {
	if len(args) < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah workspace <list|new|switch|show|persona> [args...]")
		os.Exit(2)
	}
	switch args[0] {
	case "list":
		runCtx([]string{"list"})
	case "new":
		// `leah workspace new <name> [--desc ...]` — accept positional name
		// before delegating to runCtx (which uses --name).
		runCtxNewFromWorkspace(args[1:])
	case "switch":
		runCtxSwitchFromWorkspace(args[1:])
	case "show":
		runCtx(append([]string{"show"}, args[1:]...))
	case "persona":
		runWorkspacePersona(args[1:])
	default:
		_, _ = fmt.Fprintf(os.Stderr, "leah workspace: unknown action %q\n", args[0])
		os.Exit(2)
	}
}

// runCtxNewFromWorkspace adapts positional `leah workspace new <name>` form
// to runCtx's --name flag form. Operator ergonomics: `workspace new acme`
// reads more naturally than `workspace new --name acme`.
func runCtxNewFromWorkspace(args []string) {
	fs := flag.NewFlagSet("workspace new", flag.ExitOnError)
	desc := fs.String("desc", "", "human-readable description")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah workspace new <name> [--desc ...]")
		os.Exit(2)
	}
	runCtx([]string{"new", "--name", fs.Arg(0), "--description", *desc})
}

// runCtxSwitchFromWorkspace adapts positional `leah workspace switch <name>`
// to runCtx's --name flag form.
func runCtxSwitchFromWorkspace(args []string) {
	fs := flag.NewFlagSet("workspace switch", flag.ExitOnError)
	reason := fs.String("reason", "cli", "free-form reason recorded in switch log")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah workspace switch <name> [--reason ...]")
		os.Exit(2)
	}
	runCtx([]string{"switch", "--name", fs.Arg(0), "--reason", *reason})
}

// runWorkspacePersona dispatches `leah workspace persona <show|set> ...`.
func runWorkspacePersona(args []string) {
	if len(args) < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah workspace persona <show|set> [args...]")
		os.Exit(2)
	}
	ps, err := persona.Open(memoryPath())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "open persona store: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = ps.Close() }()
	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl"), DefaultWorkspace: activeWorkspace}

	switch args[0] {
	case "show":
		fs := flag.NewFlagSet("workspace persona show", flag.ExitOnError)
		ws := fs.String("workspace", "", "target workspace (default: active)")
		jsonOut := fs.Bool("json", false, "emit json")
		_ = fs.Parse(args[1:])
		name := *ws
		if name == "" {
			name = activeWorkspace()
		}
		p, err := ps.Load(name)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "load persona: %v\n", err)
			os.Exit(1)
		}
		_ = a.Append(audit.Entry{Kind: "workspace.persona.show", BlastRadius: 0, Outcome: "success", Detail: name, Workspace: name})
		if *jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(p)
			return
		}
		fmt.Printf("workspace: %s\n", p.Workspace)
		if p.IsDefault {
			fmt.Println("(no explicit persona — falling back to package defaults)")
		}
		fmt.Printf("tone:      %s\n", p.Tone)
		fmt.Printf("signature: %s\n", p.Signature)
		fmt.Printf("voice_id:  %s\n", p.VoiceID)
	case "set":
		fs := flag.NewFlagSet("workspace persona set", flag.ExitOnError)
		ws := fs.String("workspace", "", "target workspace (default: active)")
		tone := fs.String("tone", "", "persona tone (e.g. 'formal, concise')")
		signature := fs.String("signature", "", "outbound signature (e.g. '— Tri (Acme)')")
		voiceID := fs.String("voice-id", "", "TTS voice identifier")
		_ = fs.Parse(args[1:])
		name := *ws
		if name == "" {
			name = activeWorkspace()
		}
		// Preserve unset fields by loading then overlaying (CLI partial-update
		// ergonomics: `leah workspace persona set --tone formal` shouldn't
		// blank the signature).
		cur, err := ps.Load(name)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "load persona: %v\n", err)
			os.Exit(1)
		}
		next := persona.Persona{Workspace: name, Tone: cur.Tone, Signature: cur.Signature, VoiceID: cur.VoiceID}
		if cur.IsDefault {
			// First-time set: don't carry the package-default Tone string into
			// the row (the defaults belong outside the table).
			next.Tone, next.Signature, next.VoiceID = "", "", ""
		}
		if fs.Lookup("tone").Value.String() != "" {
			next.Tone = *tone
		}
		if fs.Lookup("signature").Value.String() != "" {
			next.Signature = *signature
		}
		if fs.Lookup("voice-id").Value.String() != "" {
			next.VoiceID = *voiceID
		}
		// Soft-validate the workspace exists in the context registry — orphan
		// personas are accepted (no FK at the schema level — see
		// persona.ensureSchema rationale) but we want the operator to catch
		// typos at set-time rather than discover the wrong-workspace persona
		// is silently inactive at ask-time. WARN, do not block.
		if !workspaceExists(name) {
			_, _ = fmt.Fprintf(os.Stderr, "warn: workspace %q not registered in context table; run `leah workspace new %s` to register (persona row written anyway)\n", name, name)
		}
		if err := ps.Set(next); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "set persona: %v\n", err)
			os.Exit(1)
		}
		_ = a.Append(audit.Entry{Kind: "workspace.persona.set", BlastRadius: 1, Outcome: "success", Detail: name, Workspace: name})
		fmt.Printf("persona updated for workspace %q\n", name)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "leah workspace persona: unknown action %q\n", args[0])
		os.Exit(2)
	}
}

// workspaceExists returns true when name has a row in the context table.
// Soft-fail to false on any error path (caller emits a WARN).
func workspaceExists(name string) bool {
	mgr, err := openCtxManagerSoft()
	if err != nil {
		return false
	}
	defer func() { _ = mgr.Close() }()
	list, err := mgr.List()
	if err != nil {
		return false
	}
	for _, c := range list {
		if c.Name == name {
			return true
		}
	}
	return false
}

// activeWorkspace returns the currently active workspace name, or "default"
// on any lookup error (the ctxmgr schema seeds a default row on first open,
// so the only realistic error path is a corrupted DB — surfaced as WARN by
// callers that care, default lets daily CLI invocations keep working).
func activeWorkspace() string {
	mgr, err := openCtxManagerSoft()
	if err != nil {
		return "default"
	}
	defer func() { _ = mgr.Close() }()
	c, err := mgr.Active()
	if err != nil || c.Name == "" {
		return "default"
	}
	return c.Name
}
