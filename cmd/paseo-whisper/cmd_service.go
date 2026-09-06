package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/devXpro/paseo-whisper/internal/config"
	"github.com/devXpro/paseo-whisper/internal/service"
)

// installDir is where the binary is copied so the launchd agent points at a
// stable location rather than a build directory that may be moved or deleted.
func installDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "bin")
}

// installService copies the binary somewhere stable and registers the agent.
func installService(f flags) error {
	binary, err := stableBinary()
	if err != nil {
		return err
	}

	args := []string{"serve", "--yes"}
	if f.promptFile != "" {
		// Copy the vocabulary next to the config: an agent pointing into a
		// working tree breaks the moment that directory is moved or deleted.
		stable, err := adoptPromptFile(f.promptFile)
		if err != nil {
			return err
		}
		args = append(args, "--prompt-file", stable)
	}
	if f.saveClips {
		args = append(args, "--save-clips")
	}

	if err := service.Install(binary, args, config.Dir()); err != nil {
		return err
	}

	fmt.Printf("\n  installed as a login agent\n")
	fmt.Printf("    binary: %s\n", binary)
	fmt.Printf("    plist:  %s\n", service.PlistPath())
	fmt.Printf("    logs:   %s/service.log\n", config.Dir())
	for _, a := range args {
		if strings.HasPrefix(a, config.Dir()) {
			fmt.Printf("    terms:  %s\n", a)
		}
	}
	fmt.Println()
	fmt.Println("  it starts on login and restarts itself if it crashes")
	fmt.Println("  remove with:  paseo-whisper uninstall")
	fmt.Println()
	return nil
}

// stableBinary returns a path outside the build tree, copying the running
// binary into ~/.local/bin when needed.
func stableBinary() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return "", err
	}

	dir := installDir()
	if dir == "" {
		return self, nil
	}
	// Already running from the install location: nothing to copy.
	if filepath.Dir(self) == dir {
		return self, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(dir, "paseo-whisper")
	if err := copyExecutable(self, target); err != nil {
		return "", fmt.Errorf("copy binary to %s: %w", dir, err)
	}
	return target, nil
}

// adoptPromptFile copies the vocabulary into the config directory and returns
// the path the agent should use.
func adoptPromptFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(config.Dir(), "terms.txt")

	// Already pointing at the copy: nothing to do.
	if abs, err := filepath.Abs(path); err == nil && abs == target {
		return target, nil
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return "", err
	}
	return target, nil
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Write beside the target then rename, so a running copy is not truncated.
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func uninstallService() error {
	if err := service.Uninstall(); err != nil {
		return err
	}
	fmt.Println("\n  login agent removed")
	fmt.Printf("  the binary stays at %s/paseo-whisper\n", installDir())
	fmt.Printf("  config and models stay in %s\n\n", config.Dir())
	return nil
}

// describeService renders the agent state for doctor and status output.
func describeService() string {
	state := service.Status()
	switch {
	case !state.Installed:
		return "not installed (run: paseo-whisper install)"
	case !state.Loaded:
		return "installed but not loaded"
	default:
		return "installed and loaded"
	}
}
