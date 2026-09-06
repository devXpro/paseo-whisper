package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// clipLimit caps how many recordings are kept. Dictation clips are small, but
// an unbounded directory would grow forever.
const clipLimit = 50

// Clip is one saved recording together with what the engine made of it.
type Clip struct {
	Name     string    `json:"-"`
	Recorded time.Time `json:"recorded"`
	Text     string    `json:"text"`
	Prompt   string    `json:"prompt,omitempty"`
	Language string    `json:"language,omitempty"`
	Millis   int64     `json:"transcribe_ms"`
	Bytes    int       `json:"audio_bytes"`
}

// saveClip writes the audio next to a JSON sidecar describing the result. It
// exists so a bad transcription can be replayed against different settings
// instead of asking the user to say it again.
func (s *Server) saveClip(audio []byte, filename string, clip Clip) {
	if s.opts.ClipsDir == "" {
		return
	}
	if err := os.MkdirAll(s.opts.ClipsDir, 0o755); err != nil {
		s.log.Warn("clips directory unavailable", "err", err)
		return
	}

	stamp := clip.Recorded.Format("20060102-150405.000")
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".wav"
	}

	audioPath := filepath.Join(s.opts.ClipsDir, stamp+ext)
	if err := os.WriteFile(audioPath, audio, 0o644); err != nil {
		s.log.Warn("could not save clip audio", "err", err)
		return
	}

	clip.Bytes = len(audio)
	meta, err := json.MarshalIndent(clip, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(filepath.Join(s.opts.ClipsDir, stamp+".json"), append(meta, '\n'), 0o644); err != nil {
		s.log.Warn("could not save clip metadata", "err", err)
	}

	s.pruneClips()
}

// pruneClips deletes the oldest recordings once the directory is full.
func (s *Server) pruneClips() {
	entries, err := os.ReadDir(s.opts.ClipsDir)
	if err != nil {
		return
	}

	var stamps []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			stamps = append(stamps, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	if len(stamps) <= clipLimit {
		return
	}

	sort.Strings(stamps)
	for _, stamp := range stamps[:len(stamps)-clipLimit] {
		matches, _ := filepath.Glob(filepath.Join(s.opts.ClipsDir, stamp+".*"))
		for _, path := range matches {
			_ = os.Remove(path)
		}
	}
}

// LoadClips reads saved clips, newest first.
func LoadClips(dir string) ([]Clip, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var clips []Clip
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var clip Clip
		if err := json.Unmarshal(data, &clip); err != nil {
			continue
		}
		clip.Name = strings.TrimSuffix(e.Name(), ".json")
		clips = append(clips, clip)
	}

	sort.Slice(clips, func(i, j int) bool { return clips[i].Name > clips[j].Name })
	return clips, nil
}

// AudioPath returns the recording that belongs to a clip.
func AudioPath(dir string, clip Clip) string {
	matches, _ := filepath.Glob(filepath.Join(dir, clip.Name+".*"))
	for _, path := range matches {
		if !strings.HasSuffix(path, ".json") {
			return path
		}
	}
	return fmt.Sprintf("%s/%s.wav", dir, clip.Name)
}
