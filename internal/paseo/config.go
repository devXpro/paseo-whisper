// Package paseo reads and rewrites the Paseo daemon configuration so the user
// never has to hand-edit ~/.paseo/config.json.
package paseo

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Engine identifies which speech-to-text backend Paseo dictation uses.
type Engine string

const (
	// EngineWhisper routes dictation to an OpenAI-compatible endpoint.
	EngineWhisper Engine = "whisper"
	// EngineParakeet uses Paseo's built-in local Parakeet models.
	EngineParakeet Engine = "parakeet"
	// EngineUnknown means the config does not match either shape.
	EngineUnknown Engine = "unknown"
)

// DefaultParakeetModel is the multilingual Parakeet build. The English-only v2
// is Paseo's default and the usual reason non-English dictation fails.
const DefaultParakeetModel = "parakeet-tdt-0.6b-v3-int8"

// ErrNotFound means Paseo is not installed for this user.
var ErrNotFound = errors.New("~/.paseo/config.json not found; is Paseo installed?")

// Config is the Paseo daemon config. The raw map is preserved so rewriting one
// setting never drops keys this tool does not know about.
type Config struct {
	path string
	raw  map[string]any
}

// ConfigPath returns the location of the Paseo config file.
func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".paseo", "config.json")
}

// Load reads the Paseo config.
func Load() (*Config, error) {
	path := ConfigPath()
	if path == "" {
		return nil, ErrNotFound
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	raw := map[string]any{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &Config{path: path, raw: raw}, nil
}

// Save writes the config back, keeping a timestamped backup of the previous
// contents next to it.
func (c *Config) Save() (string, error) {
	backup := fmt.Sprintf("%s.bak.%s", c.path, time.Now().Format("20060102-150405"))
	if current, err := os.ReadFile(c.path); err == nil {
		if err := os.WriteFile(backup, current, 0o600); err != nil {
			return "", fmt.Errorf("write backup: %w", err)
		}
	}

	data, err := json.MarshalIndent(c.raw, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(c.path, append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	return backup, nil
}

// Status describes the dictation setup currently in the config.
type Status struct {
	Engine   Engine
	Provider string
	Model    string
	BaseURL  string
	Enabled  bool
}

// Status inspects features.dictation.
func (c *Config) Status() Status {
	stt := c.lookup("features", "dictation", "stt")
	if stt == nil {
		// No explicit dictation block means Paseo's own defaults apply.
		return Status{Engine: EngineParakeet, Provider: "local", Model: "parakeet-tdt-0.6b-v2-int8 (default)", Enabled: true}
	}

	s := Status{
		Provider: str(stt["provider"]),
		Model:    str(stt["model"]),
		Enabled:  true,
	}
	if enabled, ok := c.lookup("features", "dictation")["enabled"].(bool); ok {
		s.Enabled = enabled
	}

	switch s.Provider {
	case "openai":
		s.Engine = EngineWhisper
		if openai := c.lookup("providers", "openai", "stt"); openai != nil {
			s.BaseURL = str(openai["baseUrl"])
		}
	case "local", "":
		s.Engine = EngineParakeet
		if s.Provider == "" {
			s.Provider = "local"
		}
	default:
		s.Engine = EngineUnknown
	}
	return s
}

// UseWhisper points dictation at an OpenAI-compatible endpoint.
func (c *Config) UseWhisper(baseURL, modelName string) {
	c.set(map[string]any{
		"provider": "openai",
		"model":    modelName,
	}, "features", "dictation", "stt")
	c.set(true, "features", "dictation", "enabled")

	// apiKey is required by the OpenAI client but ignored by a local server.
	c.set(map[string]any{
		"apiKey":  "local",
		"baseUrl": baseURL,
	}, "providers", "openai", "stt")
}

// UseParakeet restores Paseo's built-in local engine.
func (c *Config) UseParakeet(modelName string) {
	if modelName == "" {
		modelName = DefaultParakeetModel
	}
	// The language field is deliberately omitted: Parakeet v2 is English-only
	// and v3 auto-detects, so Paseo ignores it for local models.
	c.set(map[string]any{
		"provider": "local",
		"model":    modelName,
	}, "features", "dictation", "stt")
	c.set(true, "features", "dictation", "enabled")
}

// lookup walks nested maps, returning nil when the path does not exist.
func (c *Config) lookup(keys ...string) map[string]any {
	node := c.raw
	for _, key := range keys {
		next, ok := node[key].(map[string]any)
		if !ok {
			return nil
		}
		node = next
	}
	return node
}

// set writes value at the nested path, creating intermediate maps. When value
// is a map it is merged into any existing map so sibling keys survive.
func (c *Config) set(value any, keys ...string) {
	node := c.raw
	for _, key := range keys[:len(keys)-1] {
		next, ok := node[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			node[key] = next
		}
		node = next
	}

	last := keys[len(keys)-1]
	incoming, isMap := value.(map[string]any)
	existing, hasExisting := node[last].(map[string]any)
	if isMap && hasExisting {
		for k, v := range incoming {
			existing[k] = v
		}
		return
	}
	node[last] = value
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
