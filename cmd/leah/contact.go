package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/memory"
)

// runContact dispatches `leah contact <action> ...`.
func runContact(args []string) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah contact <add|list|show> [args...]")
		return 0
	}
	if len(args) < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah contact <add|list|show> [args...]")
		return 2
	}
	store, err := openMemoryStore()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	defer func() { _ = store.Close() }()
	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl")}

	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("contact add", flag.ExitOnError)
		name := fs.String("name", "", "contact display name (required)")
		email := fs.String("email", "", "contact email")
		notes := fs.String("notes", "", "free-form notes")
		jsonOut := fs.Bool("json", false, "emit json")
		_ = fs.Parse(args[1:])
		if *name == "" {
			_, _ = fmt.Fprintln(os.Stderr, "leah contact add: --name required")
			return 2
		}
		c, err := store.AddContact(memory.Contact{Name: *name, Email: *email, Notes: *notes})
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah contact add: %v\n", err)
			return 1
		}
		_ = a.Append(audit.Entry{Kind: "contact.add", ArgsHash: c.ID, BlastRadius: 1, Outcome: "success", Detail: c.Name})
		printContact(os.Stdout, c, *jsonOut)
	case "list":
		jsonOut := hasFlag(args[1:], "--json")
		cs, err := store.ListContacts()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah contact list: %v\n", err)
			return 1
		}
		_ = a.Append(audit.Entry{Kind: "contact.list", BlastRadius: 0, Outcome: "success", Detail: fmt.Sprintf("count=%d", len(cs))})
		if jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(cs)
			return 0
		}
		if len(cs) == 0 {
			_, _ = fmt.Println("(no contacts)")
			return 0
		}
		for _, c := range cs {
			fmt.Printf("%s  %s\t<%s>\n", c.ID, c.Name, c.Email)
		}
	case "show":
		if len(args) < 2 {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah contact show <id> [--json]")
			return 2
		}
		jsonOut := hasFlag(args[2:], "--json")
		c, err := store.GetContact(args[1])
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah contact show: %v\n", err)
			return 1
		}
		_ = a.Append(audit.Entry{Kind: "contact.show", ArgsHash: c.ID, BlastRadius: 0, Outcome: "success"})
		printContact(os.Stdout, c, jsonOut)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "leah contact: unknown action %q\n", args[0])
		return 2
	}
	return 0
}

func printContact(w io.Writer, c memory.Contact, asJSON bool) {
	if asJSON {
		_ = json.NewEncoder(w).Encode(c)
		return
	}
	_, _ = fmt.Fprintf(w, "id:        %s\n", c.ID)
	_, _ = fmt.Fprintf(w, "name:      %s\n", c.Name)
	_, _ = fmt.Fprintf(w, "email:     %s\n", c.Email)
	_, _ = fmt.Fprintf(w, "notes:     %s\n", c.Notes)
	_, _ = fmt.Fprintf(w, "created:   %s\n", c.CreatedAt)
	_, _ = fmt.Fprintf(w, "updated:   %s\n", c.UpdatedAt)
}
