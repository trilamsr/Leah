package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "HUD HTTP listen addr (loopback only)")
	daemon := flag.String("daemon", "http://127.0.0.1:8080", "leah-daemon dashboard base URL")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	app := NewApp(*daemon)
	_, _ = fmt.Fprintf(os.Stdout, "leah-hud: ambient panel at http://%s/hud/ambient\n", *addr)
	if err := app.Run(ctx, *addr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah-hud: %v\n", err)
		os.Exit(1)
	}
}

// copyTo proxies io.Copy without dragging io into app.go's import set; see
// handleAmbient. Kept here so app.go imports stay tight.
func copyTo(dst io.Writer, src io.Reader) (int64, error) { return io.Copy(dst, src) }
