package paseo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// bundledCLI is where the macOS app keeps its command-line tool.
const bundledCLI = "/Applications/Paseo.app/Contents/Resources/bin/paseo"

// inheritedVars are Claude Code settings that must not leak into the daemon.
// A daemon started from a shell that exports CLAUDE_CONFIG_DIR pins that
// directory for every project (getpaseo/paseo#3618).
var inheritedVars = []string{"CLAUDE_CONFIG_DIR", "CLAUDE_PROFILE_NAME"}

// FindCLI locates the paseo command-line tool.
func FindCLI() (string, error) {
	if path, err := exec.LookPath("paseo"); err == nil {
		return path, nil
	}
	if info, err := os.Stat(bundledCLI); err == nil && !info.IsDir() {
		return bundledCLI, nil
	}
	return "", fmt.Errorf("paseo CLI not found (looked in PATH and %s)", bundledCLI)
}

// RunningInsideAgent reports whether this process was spawned by a Paseo agent.
// Restarting the daemon from there kills the very session doing the restart.
func RunningInsideAgent() bool {
	return os.Getenv("PASEO_AGENT_ID") != ""
}

// cleanEnv returns the environment minus the variables that must not be
// inherited by the daemon.
func cleanEnv() []string {
	var kept []string
	for _, entry := range os.Environ() {
		drop := false
		for _, name := range inheritedVars {
			if strings.HasPrefix(entry, name+"=") {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, entry)
		}
	}
	return kept
}

// RestartDaemon restarts the Paseo daemon with a clean environment.
func RestartDaemon(ctx context.Context) (string, error) {
	cli, err := FindCLI()
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, cli, "daemon", "restart")
	cmd.Env = cleanEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("paseo daemon restart: %w", err)
	}
	return string(out), nil
}

// WaitDaemon polls until the daemon answers again.
func WaitDaemon(ctx context.Context, timeout time.Duration) error {
	cli, err := FindCLI()
	if err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
		if exec.CommandContext(ctx, cli, "status").Run() == nil {
			return nil
		}
	}
	return fmt.Errorf("daemon did not come back within %s", timeout)
}

// WarmAgents touches every agent so their timelines load. After a restart
// agents are lazily loaded and render empty until something reads them.
func WarmAgents(ctx context.Context) (int, error) {
	cli, err := FindCLI()
	if err != nil {
		return 0, err
	}

	out, err := exec.CommandContext(ctx, cli, "ls", "-g", "--json").Output()
	if err != nil {
		return 0, fmt.Errorf("list agents: %w", err)
	}

	var agents []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &agents); err != nil {
		return 0, fmt.Errorf("parse agent list: %w", err)
	}

	warmed := 0
	for _, agent := range agents {
		if agent.ID == "" {
			continue
		}
		if err := exec.CommandContext(ctx, cli, "logs", agent.ID).Run(); err == nil {
			warmed++
		}
	}
	return warmed, nil
}
