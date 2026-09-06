// Package config holds runtime settings and remembers the user's choices
// between runs.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/devXpro/paseo-whisper/internal/model"
)

// Source names where the model should come from.
type Source string

const (
	// SourceAuto picks the best option without asking.
	SourceAuto Source = "auto"
	// SourceSuperWhisper reuses superwhisper's ggml model.
	SourceSuperWhisper Source = "superwhisper"
	// SourceMacWhisper is accepted only to give a clear error: its models are
	// CoreML and cannot be loaded by whisper.cpp.
	SourceMacWhisper Source = "macwhisper"
	// SourceDownload fetches the model from Hugging Face.
	SourceDownload Source = "download"
	// SourcePath uses an explicit file.
	SourcePath Source = "path"
)

// Config is everything the server needs to run.
type Config struct {
	ModelPath  string         `json:"model_path"`
	ModelName  string         `json:"model_name"`
	Source     Source         `json:"source"`
	LinkMode   model.LinkMode `json:"link_mode"`
	Port       int            `json:"port"`
	Threads    int            `json:"threads"`
	Language   string         `json:"language"`
	Prompt     string         `json:"prompt"`
	EnginePath string         `json:"engine_path"`
}

// Defaults returns a config with sensible values filled in.
func Defaults() Config {
	return Config{
		Source:   SourceAuto,
		LinkMode: model.LinkHard,
		Port:     8099,
		Threads:  8,
		Language: "auto",
	}
}

// Dir is where we keep the config file and any adopted models.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".paseo-whisper"
	}
	return filepath.Join(home, ".config", "paseo-whisper")
}

// ModelsDir is where downloaded or linked models live.
func ModelsDir() string {
	return filepath.Join(Dir(), "models")
}

func path() string {
	return filepath.Join(Dir(), "config.json")
}

// Load reads the saved config; a missing file is not an error.
func Load() (Config, bool) {
	data, err := os.ReadFile(path())
	if err != nil {
		return Defaults(), false
	}
	cfg := Defaults()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Defaults(), false
	}
	return cfg, cfg.ModelPath != ""
}

// Save persists the config so later runs skip the wizard.
func (c Config) Save() error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(), append(data, '\n'), 0o644)
}

// ApplyEnv lets environment variables override the stored config, which is
// what makes the service usable from launchd without editing files.
func (c *Config) ApplyEnv() {
	if v := os.Getenv("PASEO_WHISPER_MODEL"); v != "" {
		c.ModelPath = v
		c.Source = SourcePath
	}
	if v := os.Getenv("PASEO_WHISPER_ENGINE"); v != "" {
		c.EnginePath = v
	}
	if v := os.Getenv("PASEO_WHISPER_LANGUAGE"); v != "" {
		c.Language = v
	}
	if v := os.Getenv("PASEO_WHISPER_PROMPT"); v != "" {
		c.Prompt = v
	}
}

// LoadPromptFile reads a vocabulary hint file, ignoring blank lines and
// comments so the file can be commented.
func LoadPromptFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, " "), nil
}
