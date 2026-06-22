package ipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

type Handler func(ctx context.Context, req Frame) (<-chan Frame, error)

type Server struct {
	path    string
	handler Handler
}

func NewServer(socketPath string, handler Handler) *Server {
	return &Server{path: socketPath, handler: handler}
}

func (s *Server) Serve(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("mkdir socket dir: %w", err)
	}
	_ = os.Remove(s.path) // stale-socket recovery from crash (§17.8)
	ln, err := net.Listen("unix", s.path)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("chmod socket: %w", err)
	}
	go func() { <-ctx.Done(); _ = ln.Close() }()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		go s.serveConn(ctx, conn)
	}
}

func (s *Server) serveConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	for {
		req, err := ReadFrame(conn)
		if err != nil {
			return
		}
		out, err := s.handler(ctx, req)
		if err != nil {
			_ = WriteFrame(conn, Frame{Kind: "error", TurnID: req.TurnID, Seq: 0})
			continue
		}
		for f := range out {
			if err := WriteFrame(conn, f); err != nil {
				return
			}
		}
	}
}
