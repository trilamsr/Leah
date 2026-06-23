package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Handler func(ctx context.Context, req Frame) (<-chan Frame, error)

type Server struct {
	path    string
	handler Handler

	mu      sync.Mutex
	ln      net.Listener
	stopped chan struct{}
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
		_ = ln.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}
	return s.serveListener(ctx, ln)
}

func (s *Server) Stop() error {
	s.mu.Lock()
	ln := s.ln
	s.ln = nil
	stopped := s.stopped
	s.stopped = nil
	s.mu.Unlock()
	if stopped != nil {
		close(stopped)
	}
	if ln == nil {
		return nil
	}
	if err := ln.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}

func (s *Server) serveListener(ctx context.Context, ln net.Listener) error {
	stopped := make(chan struct{})
	s.mu.Lock()
	s.ln = ln
	s.stopped = stopped
	s.mu.Unlock()
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			_ = ln.Close()
		case <-stopped:
		}
	}()
	defer func() {
		s.mu.Lock()
		if s.stopped == stopped {
			s.stopped = nil
			close(stopped) // release watcher when loop exits on its own
		}
		if s.ln == ln {
			s.ln = nil
		}
		s.mu.Unlock()
		_ = ln.Close() // idempotent — Stop or permanent-error exit, either closes the FD
		<-watcherDone
	}()
	const (
		backoffMin = 5 * time.Millisecond
		backoffMax = 1 * time.Second
	)
	backoff := backoffMin
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			// EMFILE/ENFILE are transient; back off so the kernel can
			// reclaim FDs instead of crashing the daemon.
			if errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE) {
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(backoff):
				}
				if backoff < backoffMax {
					backoff *= 2
				}
				continue
			}
			return fmt.Errorf("accept: %w", err)
		}
		backoff = backoffMin
		go s.serveConn(ctx, conn)
	}
}

func (s *Server) serveConn(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()
	for {
		req, err := ReadFrame(conn)
		if err != nil {
			// Best-effort structured reject — covers oversized (frame size cap)
			// + malformed JSON + truncated body. EOF means clean conn close;
			// don't write to a half-shut conn. The error frame lets HUD
			// distinguish "your frame was bad" from "daemon died".
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return
			}
			payload, _ := json.Marshal(map[string]string{"error": err.Error()})
			if werr := WriteFrame(conn, Frame{Kind: "error", Seq: 0, Payload: payload}); werr != nil {
				return
			}
			// Recoverable read errors (e.g. malformed JSON) — KEEP the conn
			// alive so HUD can recover after sending a bad frame. Frame-size
			// cap is unrecoverable (header desync) so we close on that path.
			if strings.Contains(err.Error(), "exceeds cap") {
				return
			}
			continue
		}
		out, err := s.handler(ctx, req)
		if err != nil {
			// Best-effort error frame; if the write fails the socket is
			// half-dead — bail rather than loop on a wedged conn.
			if werr := WriteFrame(conn, Frame{Kind: "error", TurnID: req.TurnID, Seq: 0}); werr != nil {
				return
			}
			continue
		}
		if out == nil {
			// Handler may signal "no frames" with a nil channel; skip the
			// range so we don't block on a nil receive (deadlock).
			continue
		}
		for f := range out {
			if err := WriteFrame(conn, f); err != nil {
				return
			}
		}
	}
}
