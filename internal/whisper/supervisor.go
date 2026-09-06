// Package whisper runs whisper-server as a child process and keeps it alive.
package whisper

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Options configures the child whisper-server process.
type Options struct {
	Binary    string
	ModelPath string
	Threads   int
	Language  string
	Prompt    string
	LogWriter *os.File
}

// Supervisor owns the whisper-server child: it starts it on a private port,
// waits until it answers, and restarts it if it dies.
type Supervisor struct {
	opts Options
	log  *slog.Logger

	mu      sync.RWMutex
	cmd     *exec.Cmd
	port    int
	ready   bool
	stopped bool
}

// New creates a supervisor. Call Start to launch the process.
func New(opts Options, log *slog.Logger) *Supervisor {
	if opts.Threads <= 0 {
		opts.Threads = 4
	}
	if opts.Language == "" {
		opts.Language = "auto"
	}
	return &Supervisor{opts: opts, log: log}
}

// Start launches whisper-server and blocks until it responds or ctx expires.
// Loading a large model takes a few seconds, so the caller should be patient.
func (s *Supervisor) Start(ctx context.Context) error {
	port, err := freePort()
	if err != nil {
		return fmt.Errorf("allocate port for whisper-server: %w", err)
	}

	s.mu.Lock()
	s.port = port
	s.stopped = false
	s.mu.Unlock()

	if err := s.spawn(); err != nil {
		return err
	}
	if err := s.waitReady(ctx); err != nil {
		s.Stop()
		return err
	}

	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()

	go s.watch()
	return nil
}

func (s *Supervisor) spawn() error {
	s.mu.RLock()
	port := s.port
	s.mu.RUnlock()

	args := []string{
		"-m", s.opts.ModelPath,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"-t", strconv.Itoa(s.opts.Threads),
		"-l", s.opts.Language,
		"-nt", // timestamps would only pollute dictation text

		// Without this, whisper.cpp carries decoded text between requests and
		// starts dropping sentences from the middle of later clips. Each
		// dictation is independent, so no carry-over is wanted.
		"-mc", "0",
	}
	if s.opts.Prompt != "" {
		args = append(args, "--prompt", s.opts.Prompt)
	}

	cmd := exec.Command(s.opts.Binary, args...)
	if s.opts.LogWriter != nil {
		cmd.Stdout = s.opts.LogWriter
		cmd.Stderr = s.opts.LogWriter
	}
	// Own process group, so stopping us never leaves an orphaned engine.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start whisper-server: %w", err)
	}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	s.log.Info("whisper engine started", "pid", cmd.Process.Pid, "port", port, "model", s.opts.ModelPath)
	return nil
}

// waitReady polls the child until it accepts requests.
func (s *Supervisor) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(3 * time.Minute)
	client := &http.Client{Timeout: 2 * time.Second}

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("whisper-server did not become ready in time")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}

		// A dead child will never become ready; fail fast instead of looping.
		s.mu.RLock()
		cmd := s.cmd
		s.mu.RUnlock()
		if cmd != nil && cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return fmt.Errorf("whisper-server exited during startup: %s", cmd.ProcessState)
		}

		conn, err := net.DialTimeout("tcp", s.Addr(), time.Second)
		if err != nil {
			continue
		}
		conn.Close()

		// The port is open before the model finishes loading, so confirm the
		// HTTP layer actually answers.
		resp, err := client.Get("http://" + s.Addr() + "/")
		if err == nil {
			resp.Body.Close()
			return nil
		}
	}
}

// watch restarts the child if it exits unexpectedly.
func (s *Supervisor) watch() {
	for {
		s.mu.RLock()
		cmd, stopped := s.cmd, s.stopped
		s.mu.RUnlock()
		if stopped || cmd == nil {
			return
		}

		err := cmd.Wait()

		s.mu.RLock()
		stopped = s.stopped
		s.mu.RUnlock()
		if stopped {
			return
		}

		s.mu.Lock()
		s.ready = false
		s.mu.Unlock()

		s.log.Error("whisper engine exited, restarting", "err", err)
		time.Sleep(2 * time.Second)

		if err := s.spawn(); err != nil {
			s.log.Error("restart failed", "err", err)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		if err := s.waitReady(ctx); err != nil {
			s.log.Error("engine did not recover", "err", err)
			cancel()
			return
		}
		cancel()

		s.mu.Lock()
		s.ready = true
		s.mu.Unlock()
		s.log.Info("whisper engine recovered")
	}
}

// Addr is the host:port of the child process.
func (s *Supervisor) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return "127.0.0.1:" + strconv.Itoa(s.port)
}

// Ready reports whether the engine can serve requests right now.
func (s *Supervisor) Ready() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ready
}

// Stop terminates the child process and its group.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	s.stopped = true
	s.ready = false
	cmd := s.cmd
	s.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}
	// Negative pid signals the whole process group.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// freePort asks the kernel for an unused port.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
