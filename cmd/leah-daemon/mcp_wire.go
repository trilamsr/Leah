package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/trilam/leah/internal/mcp"
)

// startMCPPublish binds the outbound MCP Unix socket when LEAH_MCP_PUBLISH=1.
// Default OFF — Phase 3 ship gate. ServePublish removes the socket file on
// ctx cancel; ErrPublishDisabled is a no-op exit (gate off, no bind).
func startMCPPublish(ctx context.Context, errOut io.Writer) {
	startMCPPublishAt(ctx, errOut, mcp.DefaultPublishSocketPath())
}

func startMCPPublishAt(ctx context.Context, errOut io.Writer, sockPath string) {
	go func() {
		err := mcp.ServePublish(ctx, sockPath)
		if err != nil && !errors.Is(err, mcp.ErrPublishDisabled) && ctx.Err() == nil {
			_, _ = fmt.Fprintf(errOut, "leah-daemon: mcp publish: %v\n", err)
		}
	}()
}
