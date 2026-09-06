// Package model resolves which Whisper model file the server should load,
// downloading or linking one when necessary.
//
// There is deliberately no model choice: large-v3-turbo wins on both axes we
// care about. Benchmarked against large-v3 on Russian technical speech it was
// 1.8x faster (8.2x vs 4.6x realtime) at equal accuracy, so offering a menu
// would only invite worse decisions.
package model

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devXpro/paseo-whisper/internal/discover"
)

// Turbo is the only model this tool manages.
var Turbo = Entry{
	ID:          "large-v3-turbo",
	SizeMB:      1550,
	Description: "Whisper large-v3-turbo: near large-v3 accuracy at real-time speed.",
	URL:         "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo.bin",
}

// Entry is a downloadable model.
type Entry struct {
	ID          string
	SizeMB      int64
	Description string
	URL         string
}

// LinkMode decides how a model discovered elsewhere becomes ours.
type LinkMode string

const (
	// LinkHard creates a hard link: no extra disk use, and the file survives
	// the owning application deleting its copy.
	LinkHard LinkMode = "hardlink"
	// LinkCopy duplicates the file, costing disk but fully independent.
	LinkCopy LinkMode = "copy"
	// LinkNone uses the file where it already lives.
	LinkNone LinkMode = "none"
)

// ErrNoModel means nothing was found and downloading was not permitted.
var ErrNoModel = errors.New("no usable ggml model found")

// Adopt places src into dir according to mode and returns the path to use.
// A hard link silently degrades to a copy across filesystems.
func Adopt(src, dir string, mode LinkMode) (string, error) {
	if mode == LinkNone {
		return src, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	dst := filepath.Join(dir, filepath.Base(src))
	if existing, err := os.Stat(dst); err == nil && existing.Size() > 0 {
		return dst, nil // already adopted
	}

	if mode == LinkHard {
		if err := os.Link(src, dst); err == nil {
			return dst, nil
		}
		// Cross-device or unsupported: fall through to copying.
	}

	if err := copyFile(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".partial"
	out, err := os.Create(tmp)
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

// Download fetches the turbo model into dir, reporting progress. An interrupted
// download leaves only a .partial file behind, so a retry starts clean.
func Download(ctx context.Context, dir string, progress func(done, total int64)) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	dst := filepath.Join(dir, "ggml-"+Turbo.ID+".bin")
	if info, err := os.Stat(dst); err == nil && info.Size() > 10<<20 {
		return dst, nil // already downloaded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Turbo.URL, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 2 * time.Hour}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: unexpected status %s", Turbo.ID, resp.Status)
	}

	tmp := dst + ".partial"
	out, err := os.Create(tmp)
	if err != nil {
		return "", err
	}

	var reader io.Reader = resp.Body
	if progress != nil {
		reader = &progressReader{r: resp.Body, total: resp.ContentLength, report: progress}
	}

	if _, err := io.Copy(out, reader); err != nil {
		out.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return dst, nil
}

type progressReader struct {
	r      io.Reader
	total  int64
	done   int64
	report func(done, total int64)
	last   time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	// Throttle so progress output stays readable in a terminal.
	if time.Since(p.last) > 500*time.Millisecond || err == io.EOF {
		p.last = time.Now()
		p.report(p.done, p.total)
	}
	return n, err
}

// Validate rejects paths that whisper.cpp would fail on anyway, with a message
// explaining what to do instead.
func Validate(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if strings.Contains(path, "whisperkit") || strings.Contains(path, "mlmodelc") {
			return errors.New("this is a CoreML/WhisperKit model (MacWhisper); whisper.cpp needs a ggml .bin file")
		}
		return errors.New("expected a ggml .bin file, got a directory")
	}
	if info.Size() < 10<<20 {
		return fmt.Errorf("file is only %d bytes; a real ggml model is hundreds of MB", info.Size())
	}
	return nil
}

// BestDiscovered picks a usable model already on this machine, preferring a
// turbo build over anything else.
func BestDiscovered(models []discover.Model) (discover.Model, bool) {
	var fallback discover.Model
	var haveFallback bool

	for _, m := range models {
		if !m.Usable {
			continue
		}
		if strings.Contains(strings.ToLower(m.Name), "turbo") {
			return m, true
		}
		if !haveFallback {
			fallback, haveFallback = m, true
		}
	}
	return fallback, haveFallback
}
