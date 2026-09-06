package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/devXpro/paseo-whisper/internal/config"
	"github.com/devXpro/paseo-whisper/internal/paseo"
	"github.com/devXpro/paseo-whisper/internal/service"
)

// paseoCommand implements `paseo-whisper paseo <status|use|restart>`.
func paseoCommand(ctx context.Context, sub string, rest []string, f flags) error {
	switch sub {
	case "", "status":
		return paseoStatus()
	case "use":
		return paseoUse(ctx, argOr(rest, ""), f)
	case "restart":
		return paseoRestart(ctx)
	default:
		return fmt.Errorf("unknown paseo command %q (try: status, use, restart)", sub)
	}
}

func paseoStatus() error {
	cfg, err := paseo.Load()
	if err != nil {
		return err
	}
	s := cfg.Status()

	fmt.Printf("\n  Paseo dictation\n")
	fmt.Printf("  ───────────────\n")
	fmt.Printf("  engine:    %s\n", s.Engine)
	fmt.Printf("  provider:  %s\n", s.Provider)
	if s.Model != "" {
		fmt.Printf("  model:     %s\n", s.Model)
	}
	if s.BaseURL != "" {
		fmt.Printf("  endpoint:  %s  [%s]\n", s.BaseURL, probe(s.BaseURL))
	}
	if !s.Enabled {
		fmt.Printf("  enabled:   no\n")
	}
	fmt.Printf("  config:    %s\n\n", paseo.ConfigPath())

	switch s.Engine {
	case paseo.EngineWhisper:
		fmt.Println("  switch back:  paseo-whisper paseo use parakeet")
	case paseo.EngineParakeet:
		fmt.Println("  switch:       paseo-whisper paseo use whisper")
	}
	fmt.Println()
	return nil
}

// probe reports whether our own server is answering, so a misconfigured
// endpoint is obvious before the user tries to dictate.
func probe(baseURL string) string {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL + "/models")
	if err != nil {
		return "not responding"
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return "reachable"
}

func paseoUse(ctx context.Context, engine string, f flags) error {
	pc, err := paseo.Load()
	if err != nil {
		return err
	}

	var summary string
	switch engine {
	case "whisper":
		own, _ := config.Load()
		port := own.Port
		if f.port != 0 {
			port = f.port
		}
		if port == 0 {
			port = 8099
		}
		baseURL := fmt.Sprintf("http://127.0.0.1:%d/v1", port)

		name := own.ModelName
		if name == "" {
			name = "whisper-1"
		}
		pc.UseWhisper(baseURL, name)
		summary = fmt.Sprintf("whisper via %s (model %s)", baseURL, name)

		// Dictation now depends on this server, so make sure it will be
		// running after the next reboot rather than failing silently.
		ensureAutostart(baseURL, f)

	case "parakeet", "local":
		name := f.modelPath // reuse --model as the Parakeet model id
		if name == "" {
			name = paseo.DefaultParakeetModel
		}
		pc.UseParakeet(name)
		summary = fmt.Sprintf("parakeet (model %s)", name)

	default:
		return fmt.Errorf("use what? try: paseo-whisper paseo use whisper|parakeet")
	}

	backup, err := pc.Save()
	if err != nil {
		return fmt.Errorf("write Paseo config: %w", err)
	}

	fmt.Printf("\n  Paseo dictation set to: %s\n", summary)
	fmt.Printf("  backup: %s\n\n", backup)

	if !f.restart {
		printRestartHint()
		return nil
	}
	return paseoRestart(ctx)
}

// ensureAutostart installs the login agent when Paseo has just been pointed at
// a server that is not set up to run on its own.
func ensureAutostart(baseURL string, f flags) {
	state := service.Status()
	reachable := probe(baseURL) == "reachable"

	if state.Installed && state.Loaded {
		if !reachable {
			fmt.Printf("  the login agent is installed but not answering yet; it should come up shortly\n")
		}
		return
	}

	if f.noAutostart {
		if !reachable {
			fmt.Printf("\n  warning: nothing is answering on %s and autostart is disabled.\n", baseURL)
			fmt.Printf("           dictation will fail until you run: paseo-whisper serve\n")
		}
		return
	}

	fmt.Println("  dictation now depends on this server, installing it as a login agent")
	if err := installService(f); err != nil {
		fmt.Printf("  could not install the login agent: %v\n", err)
		fmt.Printf("  start the server manually, or dictation will fail after a reboot\n")
		return
	}
	waitForServer(baseURL)
}

// waitForServer gives a freshly loaded agent a moment to bind, so the user is
// told the truth about whether dictation will work right now.
func waitForServer(baseURL string) {
	for i := 0; i < 30; i++ {
		if probe(baseURL) == "reachable" {
			fmt.Printf("  server is up at %s\n\n", baseURL)
			return
		}
		time.Sleep(time.Second)
	}
	fmt.Printf("  server has not answered yet; check: paseo-whisper doctor\n\n")
}

func printRestartHint() {
	fmt.Println("  Speech settings are read at daemon startup, so a restart is required:")
	fmt.Println("    paseo-whisper paseo restart")
	if paseo.RunningInsideAgent() {
		fmt.Println()
		fmt.Println("  Careful: this process is running inside a Paseo agent.")
		fmt.Println("  Restarting the daemon will end that session (history is kept).")
	}
	fmt.Println()
}

func paseoRestart(ctx context.Context) error {
	if paseo.RunningInsideAgent() {
		fmt.Println("\n  Restarting from inside a Paseo agent ends this session.")
		fmt.Println("  The chat history survives; reopen it afterwards.")
	}

	fmt.Println("\n  restarting the Paseo daemon with a clean environment...")
	out, err := paseo.RestartDaemon(ctx)
	if out != "" {
		fmt.Printf("%s", out)
	}
	if err != nil {
		return err
	}

	if err := paseo.WaitDaemon(ctx, 60*time.Second); err != nil {
		return err
	}

	fmt.Println("  warming agents so their timelines are not empty...")
	warmed, err := paseo.WarmAgents(ctx)
	if err != nil {
		fmt.Printf("  could not warm agents: %v\n", err)
	} else {
		fmt.Printf("  warmed %d agents\n", warmed)
	}

	fmt.Println("\n  done. On mobile, quit and reopen the Paseo app: the relay")
	fmt.Println("  connection was dropped and the client reconnects with a stale cache.")
	fmt.Println()
	return nil
}
