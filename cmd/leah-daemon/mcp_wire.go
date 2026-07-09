package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/trilam/leah/internal/mcp"
	"github.com/trilam/leah/internal/obs"
)

// startMCPPublish binds the outbound MCP Unix socket when LEAH_MCP_PUBLISH=1.
// Default OFF. ServePublish removes the socket file on ctx cancel;
// ErrPublishDisabled is a no-op exit (gate off, no bind).
func startMCPPublish(ctx context.Context, lg *slog.Logger, registry *obs.Registry, errOut io.Writer) {
	_ = startMCPPublishAt(ctx, lg, registry, errOut, mcp.DefaultPublishSocketPath())
}

// startMCPPublishAt returns a done-channel closed when the publish goroutine
// exits — tests gate the gate-off negative assertion ("socket not created")
// on this instead of a wall-clock sleep, since ServePublish returns
// synchronously with ErrPublishDisabled when the gate is off.
func startMCPPublishAt(ctx context.Context, lg *slog.Logger, registry *obs.Registry, errOut io.Writer, sockPath string) <-chan struct{} {
	done := make(chan struct{})
	obs.SafeGo(lg, registry, "mcp-publish", func() {
		defer close(done)
		err := mcp.ServePublish(ctx, sockPath)
		if err != nil && !errors.Is(err, mcp.ErrPublishDisabled) && ctx.Err() == nil {
			_, _ = fmt.Fprintf(errOut, "leah-daemon: mcp publish: %v\n", err)
		}
	})
	return done
}
