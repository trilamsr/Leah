package main

import (
	"fmt"
	"os"
)

const version = "0.0.1-mvp5"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version", "-v", "--version":
		fmt.Println(version)
		return
	case "ask", "ship", "review", "status":
		fmt.Fprintf(os.Stderr, "subcommand %q not yet implemented\n", os.Args[1])
		os.Exit(2)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Leah — personal AI chief-of-staff (MVP-5)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "usage: leah <command> [args...]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  ask \"<query>\"        direct query to Reasoner")
	fmt.Fprintln(os.Stderr, "  ship \"<intent>\"      file regatta issue + watch + narrate")
	fmt.Fprintln(os.Stderr, "  review <pr#>         independent reviewer subagent on PR")
	fmt.Fprintln(os.Stderr, "  status               recent activity from audit log")
	fmt.Fprintln(os.Stderr, "  version              show version")
}
