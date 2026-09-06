// Package terms mines the user's own chat transcripts for domain vocabulary,
// so the transcription prompt reflects how they actually speak rather than a
// generic word list.
package terms

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Scan reads Claude Code transcripts and returns the vocabulary worth feeding
// to Whisper, most frequent first.
type Scan struct {
	// Roots to search. Empty means the default Claude locations.
	Roots []string
	// MinCount drops terms seen fewer times than this.
	MinCount int
	// Limit caps how many terms are returned.
	Limit int
	// MaxFilesPerRoot bounds the work on very large histories.
	MaxFilesPerRoot int
}

// Term is a word the user says often.
type Term struct {
	Word  string
	Count int
}

var (
	latinWord = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9]{2,19}\b`)
	cyrWord   = regexp.MustCompile(`[а-яё]{4,20}`)
)

// noiseMarkers identify transcript entries written by tooling rather than by
// the user. Their vocabulary would swamp everything else.
var noiseMarkers = []string{
	"Caveat:", "<command-name>", "<system-reminder>", "Request interrupted",
	"<local-command", "tool_use_id", "<task-notification>", "This is an automated",
}

// DefaultRoots returns the Claude transcript directories on this machine,
// including the per-project config dirs used when CLAUDE_CONFIG_DIR is set.
func DefaultRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	roots := []string{filepath.Join(home, ".claude", "projects")}

	// Per-project isolation puts transcripts under <project>/.claude-data.
	matches, _ := filepath.Glob(filepath.Join(home, "projects", "*", ".claude-data", "projects"))
	roots = append(roots, matches...)

	var existing []string
	for _, r := range roots {
		if info, err := os.Stat(r); err == nil && info.IsDir() {
			existing = append(existing, r)
		}
	}
	return existing
}

// Run performs the scan.
func (s Scan) Run() ([]Term, int, error) {
	if len(s.Roots) == 0 {
		s.Roots = DefaultRoots()
	}
	if s.MinCount <= 0 {
		s.MinCount = 8
	}
	if s.Limit <= 0 {
		s.Limit = 60
	}
	if s.MaxFilesPerRoot <= 0 {
		s.MaxFilesPerRoot = 100
	}

	counts := map[string]int{}
	messages := 0

	for _, root := range s.Roots {
		files, err := recentFiles(root, s.MaxFilesPerRoot)
		if err != nil {
			continue
		}
		for _, path := range files {
			messages += scanFile(path, counts)
		}
	}

	var terms []Term
	for word, count := range counts {
		if count < s.MinCount || stopWords[word] {
			continue
		}
		terms = append(terms, Term{Word: word, Count: count})
	}

	// Frequent first, alphabetical within a count so output is stable.
	sort.Slice(terms, func(i, j int) bool {
		if terms[i].Count != terms[j].Count {
			return terms[i].Count > terms[j].Count
		}
		return terms[i].Word < terms[j].Word
	})

	if len(terms) > s.Limit {
		terms = terms[:s.Limit]
	}
	return terms, messages, nil
}

// recentFiles lists the newest transcripts, since old projects say less about
// what the user works on now.
func recentFiles(root string, limit int) ([]string, error) {
	type entry struct {
		path string
		mod  int64
	}
	var found []entry

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil //nolint:nilerr // unreadable entries are skipped
		}
		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr
		}
		found = append(found, entry{path, info.ModTime().Unix()})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(found, func(i, j int) bool { return found[i].mod > found[j].mod })
	if len(found) > limit {
		found = found[:limit]
	}

	paths := make([]string, len(found))
	for i, e := range found {
		paths[i] = e.path
	}
	return paths, nil
}

// scanFile counts words in the user's own messages and returns how many were
// examined.
func scanFile(path string, counts map[string]int) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8<<20)

	seen := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if !strings.Contains(string(line[:min(len(line), 200)]), `"user"`) {
			continue
		}

		var record struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(line, &record); err != nil || record.Type != "user" {
			continue
		}

		text := extractText(record.Message.Content)
		if text == "" || len(text) > 2000 || isNoise(text) {
			continue
		}
		seen++
		countWords(text, counts)
	}
	return seen
}

// extractText handles both plain string content and the structured block form.
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}

	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, " ")
}

func isNoise(text string) bool {
	for _, marker := range noiseMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	// Pasted logs and command output are not speech.
	return strings.Count(text, "\n") > 8
}

func countWords(text string, counts map[string]int) {
	lower := strings.ToLower(text)
	for _, w := range latinWord.FindAllString(lower, -1) {
		counts[w]++
	}
	for _, w := range cyrWord.FindAllString(lower, -1) {
		counts[w]++
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
