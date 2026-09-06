// Package service installs paseo-whisper as a launchd agent so the
// transcription server is running whenever Paseo might need it.
package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Label is the launchd identifier for the agent.
const Label = "com.devxpro.paseo-whisper"

// State describes how the agent is set up right now. The three fields fail
// independently: a plist can exist without being loaded, and a loaded agent
// can still have a dead process.
type State struct {
	Installed  bool // plist exists on disk
	Loaded     bool // launchd knows about it
	PlistPath  string
	BinaryPath string
}

// PlistPath is where the agent definition lives.
func PlistPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
}

// Status reports the current installation state.
func Status() State {
	path := PlistPath()
	state := State{PlistPath: path}

	if data, err := os.ReadFile(path); err == nil {
		state.Installed = true
		state.BinaryPath = binaryFromPlist(string(data))
	}

	out, err := exec.Command("launchctl", "list").Output()
	if err == nil && strings.Contains(string(out), Label) {
		state.Loaded = true
	}
	return state
}

// Install writes the agent and loads it. Passing the current binary path keeps
// the agent pointing at whatever build the user is running.
func Install(binary string, args []string, logDir string) error {
	if binary == "" {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locate own binary: %w", err)
		}
		binary, err = filepath.EvalSymlinks(self)
		if err != nil {
			return fmt.Errorf("resolve own binary: %w", err)
		}
	}

	path := PlistPath()
	if path == "" {
		return fmt.Errorf("cannot determine home directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(renderPlist(binary, args, logDir)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	// Unload first so a re-install picks up the new definition.
	_ = exec.Command("launchctl", "unload", path).Run()
	if out, err := exec.Command("launchctl", "load", "-w", path).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Uninstall stops the agent and removes its definition.
func Uninstall() error {
	path := PlistPath()
	if path == "" {
		return fmt.Errorf("cannot determine home directory")
	}
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Restart bounces the agent, which is how a config change takes effect.
func Restart() error {
	path := PlistPath()
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("agent is not installed")
	}
	_ = exec.Command("launchctl", "unload", path).Run()
	if out, err := exec.Command("launchctl", "load", "-w", path).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// renderPlist builds the agent definition.
//
// KeepAlive is conditional on SuccessfulExit=false: a crash is restarted, but a
// clean shutdown (the user stopping it deliberately) is respected.
func renderPlist(binary string, args []string, logDir string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>` + Label + `</string>

    <key>ProgramArguments</key>
    <array>
`)
	b.WriteString("        <string>" + escape(binary) + "</string>\n")
	for _, a := range args {
		b.WriteString("        <string>" + escape(a) + "</string>\n")
	}
	b.WriteString(`    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>

    <key>ThrottleInterval</key>
    <integer>10</integer>

    <key>ProcessType</key>
    <string>Background</string>

    <key>StandardOutPath</key>
    <string>` + escape(filepath.Join(logDir, "service.log")) + `</string>

    <key>StandardErrorPath</key>
    <string>` + escape(filepath.Join(logDir, "service.log")) + `</string>
</dict>
</plist>
`)
	return b.String()
}

// binaryFromPlist pulls the executable path back out of an installed agent, so
// status output can show which build is actually being run.
func binaryFromPlist(plist string) string {
	start := strings.Index(plist, "<array>")
	if start < 0 {
		return ""
	}
	rest := plist[start:]
	open := strings.Index(rest, "<string>")
	if open < 0 {
		return ""
	}
	rest = rest[open+len("<string>"):]
	close := strings.Index(rest, "</string>")
	if close < 0 {
		return ""
	}
	return unescape(rest[:close])
}

func escape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func unescape(s string) string {
	r := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">")
	return r.Replace(s)
}
