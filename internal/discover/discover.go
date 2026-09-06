// Package discover locates the whisper.cpp engine and any Whisper models that
// other applications on this machine have already downloaded.
package discover

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ModelFormat distinguishes model files we can actually feed to whisper.cpp
// from the ones we can only report to the user.
type ModelFormat string

const (
	// FormatGGML is the native whisper.cpp format. Usable directly.
	FormatGGML ModelFormat = "ggml"
	// FormatCoreML is Apple's WhisperKit layout. Reported but not usable.
	FormatCoreML ModelFormat = "coreml"
)

// Model is a Whisper model found somewhere on this machine.
type Model struct {
	Path   string
	Owner  string // application that downloaded it
	Format ModelFormat
	SizeMB int64
	Name   string
	Usable bool
	Reason string // why it is not usable, when Usable is false
}

// Engine is a whisper.cpp server binary.
type Engine struct {
	Path    string
	Version string
	Source  string // how it was found: homebrew, PATH, bundled
}

// minCoreMLSizeMB filters out the auxiliary blobs inside WhisperKit bundles,
// which are quantisation and diarisation artefacts rather than speech models.
const minCoreMLSizeMB = 200

// knownModelDirs lists where third-party apps keep their models, in the order
// we prefer them. Paths are relative to the user's home directory.
var knownModelDirs = []struct {
	owner string
	path  string
}{
	{"superwhisper", "Library/Application Support/superwhisper"},
	{"MacWhisper", "Library/Application Support/MacWhisper/models"},
	{"Raycast", "Library/Application Support/com.raycast.macos/extensions"},
}

// FindEngine locates a whisper-server binary, preferring an explicit override.
func FindEngine(override string) (*Engine, error) {
	if override != "" {
		if err := checkExecutable(override); err != nil {
			return nil, err
		}
		return &Engine{Path: override, Version: engineVersion(override), Source: "override"}, nil
	}

	// A binary next to our own is the most predictable choice.
	if self, err := os.Executable(); err == nil {
		local := filepath.Join(filepath.Dir(self), "whisper-server")
		if checkExecutable(local) == nil {
			return &Engine{Path: local, Version: engineVersion(local), Source: "bundled"}, nil
		}
	}

	if path, err := exec.LookPath("whisper-server"); err == nil {
		source := "PATH"
		if strings.Contains(path, "/homebrew/") || strings.Contains(path, "/usr/local/") {
			source = "homebrew"
		}
		return &Engine{Path: path, Version: engineVersion(path), Source: source}, nil
	}

	return nil, ErrEngineNotFound
}

func checkExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return ErrNotExecutable
	}
	return nil
}

// engineVersion asks Homebrew what it installed. whisper-server itself has no
// --version flag, so an empty string simply means "unknown".
func engineVersion(path string) string {
	if !strings.Contains(path, "/homebrew/") && !strings.Contains(path, "/usr/local/") {
		return ""
	}
	out, err := exec.Command("brew", "list", "--versions", "whisper-cpp").Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

// Models walks the known locations and returns everything it finds, usable
// entries first so callers can present a sensible default.
func Models() []Model {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var found []Model
	for _, dir := range knownModelDirs {
		root := filepath.Join(home, dir.path)
		if _, err := os.Stat(root); err != nil {
			continue
		}
		found = append(found, scanDir(root, dir.owner)...)
	}

	// Usable models first; within a group, larger files rank higher because
	// they are the more capable variants.
	sortModels(found)
	return found
}

// scanDir looks for ggml files and for the CoreML layout, capping the walk
// depth so a stray huge directory cannot stall startup.
func scanDir(root, owner string) []Model {
	var found []Model
	seenCoreML := map[string]bool{}

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // unreadable subtrees are simply skipped
		}
		if depth(root, path) > 6 {
			return filepath.SkipDir
		}

		name := d.Name()
		if d.IsDir() {
			// A .mlmodelc bundle marks a WhisperKit model; record its parent
			// once and stop descending into the weights.
			if strings.HasSuffix(name, ".mlmodelc") {
				parent := filepath.Dir(path)
				if !seenCoreML[parent] {
					seenCoreML[parent] = true
					// WhisperKit bundles also ship small quantisation and
					// diarisation blobs; only speech models are worth naming.
					if size := dirSizeMB(parent); size >= minCoreMLSizeMB {
						found = append(found, Model{
							Path:   parent,
							Owner:  owner,
							Format: FormatCoreML,
							Name:   filepath.Base(parent),
							SizeMB: size,
							Usable: false,
							Reason: "CoreML/WhisperKit format, which whisper.cpp cannot load",
						})
					}
				}
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(name, ".bin") || !strings.HasPrefix(name, "ggml") {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() < 10<<20 { // ignore stubs and partial files
			return nil
		}
		found = append(found, Model{
			Path:   path,
			Owner:  owner,
			Format: FormatGGML,
			Name:   strings.TrimSuffix(strings.TrimPrefix(name, "ggml-"), ".bin"),
			SizeMB: info.Size() >> 20,
			Usable: true,
		})
		return nil
	})

	return found
}

func depth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator))
}

func dirSizeMB(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // best-effort size reporting
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total >> 20
}

func sortModels(models []Model) {
	for i := 1; i < len(models); i++ {
		for j := i; j > 0 && less(models[j], models[j-1]); j-- {
			models[j], models[j-1] = models[j-1], models[j]
		}
	}
}

func less(a, b Model) bool {
	if a.Usable != b.Usable {
		return a.Usable
	}
	return a.SizeMB > b.SizeMB
}
