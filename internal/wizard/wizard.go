// Package wizard walks the user through obtaining a Whisper model on first run.
package wizard

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/devXpro/paseo-whisper/internal/config"
	"github.com/devXpro/paseo-whisper/internal/discover"
	"github.com/devXpro/paseo-whisper/internal/model"
)

// Run asks how to obtain the model and returns the completed config. The model
// itself is not up for debate: only where the file comes from.
func Run(ctx context.Context, cfg config.Config, engine *discover.Engine, found []discover.Model) (config.Config, error) {
	in := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("  paseo-whisper setup")
	fmt.Println("  ───────────────────")
	fmt.Printf("  engine: %s", engine.Path)
	if engine.Version != "" {
		fmt.Printf(" (whisper-cpp %s)", engine.Version)
	}
	fmt.Printf("  [%s]\n", engine.Source)
	fmt.Printf("  model:  %s\n\n", model.Turbo.ID)

	reportFindings(os.Stdout, found)

	reusable, canReuse := model.BestDiscovered(found)
	choices := buildChoices(reusable, canReuse)
	printChoices(choices)

	pick, err := ask(in, len(choices))
	if err != nil {
		return cfg, err
	}

	switch choices[pick].kind {
	case choiceExisting:
		linkMode, err := askLinkMode(in)
		if err != nil {
			return cfg, err
		}
		cfg.LinkMode = linkMode
		path, err := model.Adopt(reusable.Path, config.ModelsDir(), linkMode)
		if err != nil {
			return cfg, fmt.Errorf("adopt model: %w", err)
		}
		cfg.ModelPath, cfg.ModelName = path, reusable.Name

	case choiceDownload:
		fmt.Printf("\n  downloading %s (~%d MB)...\n", model.Turbo.ID, model.Turbo.SizeMB)
		path, err := model.Download(ctx, config.ModelsDir(), printProgress)
		if err != nil {
			return cfg, fmt.Errorf("download model: %w", err)
		}
		fmt.Println()
		cfg.ModelPath, cfg.ModelName = path, model.Turbo.ID
		cfg.LinkMode = model.LinkNone

	case choiceCustom:
		fmt.Print("\n  path to a ggml .bin file: ")
		line, err := in.ReadString('\n')
		if err != nil {
			return cfg, err
		}
		path := strings.TrimSpace(line)
		if err := model.Validate(path); err != nil {
			return cfg, err
		}
		cfg.ModelPath = path
		cfg.ModelName = strings.TrimSuffix(strings.TrimPrefix(baseName(path), "ggml-"), ".bin")
		cfg.LinkMode = model.LinkNone
	}

	cfg.EnginePath = engine.Path
	fmt.Printf("\n  model ready: %s\n\n", cfg.ModelPath)
	return cfg, nil
}

// reportFindings prints what is installed, including models we cannot use, so
// the user is not left wondering why MacWhisper was ignored.
func reportFindings(w io.Writer, found []discover.Model) {
	if len(found) == 0 {
		fmt.Fprintln(w, "  No existing Whisper models found on this machine.")
		fmt.Fprintln(w)
		return
	}

	fmt.Fprintln(w, "  Models found on this machine:")
	for _, m := range found {
		mark := "usable"
		if !m.Usable {
			mark = "not usable"
		}
		fmt.Fprintf(w, "    • %-22s %-14s %5d MB   [%s]\n", m.Name, m.Owner, m.SizeMB, mark)
		if !m.Usable {
			fmt.Fprintf(w, "      %s\n", m.Reason)
		}
	}
	fmt.Fprintln(w)
}

type choiceKind int

const (
	choiceExisting choiceKind = iota
	choiceDownload
	choiceCustom
)

type choice struct {
	kind      choiceKind
	label     string
	hint      string
	recommend bool
}

func buildChoices(reusable discover.Model, canReuse bool) []choice {
	var choices []choice
	if canReuse {
		choices = append(choices, choice{
			kind:      choiceExisting,
			label:     fmt.Sprintf("Reuse %s from %s (%d MB)", reusable.Name, reusable.Owner, reusable.SizeMB),
			hint:      "no download, no extra disk space",
			recommend: true,
		})
	}
	choices = append(choices, choice{
		kind:      choiceDownload,
		label:     fmt.Sprintf("Download %s (~%d MB)", model.Turbo.ID, model.Turbo.SizeMB),
		hint:      "independent of other apps on this machine",
		recommend: !canReuse,
	})
	return append(choices, choice{
		kind:  choiceCustom,
		label: "Use a ggml file I specify",
		hint:  "point at your own .bin",
	})
}

func printChoices(choices []choice) {
	fmt.Println("  Where should the model come from?")
	fmt.Println()
	for i, c := range choices {
		suffix := ""
		if c.recommend {
			suffix = "  ← recommended"
		}
		fmt.Printf("   %d) %s%s\n", i+1, c.label, suffix)
		if c.hint != "" {
			fmt.Printf("      %s\n", c.hint)
		}
	}
	fmt.Println()
}

func ask(in *bufio.Reader, max int) (int, error) {
	for {
		fmt.Printf("  choice [1-%d]: ", max)
		line, err := in.ReadString('\n')
		if err != nil {
			return 0, err
		}
		n, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || n < 1 || n > max {
			fmt.Println("  please enter one of the listed numbers")
			continue
		}
		return n - 1, nil
	}
}

func askLinkMode(in *bufio.Reader) (model.LinkMode, error) {
	fmt.Println()
	fmt.Println("  That model belongs to another app. How should we reference it?")
	fmt.Println()
	fmt.Println("   1) Hard link  ← recommended")
	fmt.Println("      Costs no extra disk space, and the file survives the other app deleting it.")
	fmt.Println("   2) Copy")
	fmt.Println("      Fully independent, but uses the space twice.")
	fmt.Println("   3) Use in place")
	fmt.Println("      Nothing is created, but the model vanishes if that app removes it.")
	fmt.Println()

	pick, err := ask(in, 3)
	if err != nil {
		return model.LinkHard, err
	}
	return [...]model.LinkMode{model.LinkHard, model.LinkCopy, model.LinkNone}[pick], nil
}

func printProgress(done, total int64) {
	if total <= 0 {
		fmt.Printf("\r  %d MB", done>>20)
		return
	}
	fmt.Printf("\r  %d / %d MB (%d%%)", done>>20, total>>20, done*100/total)
}

func baseName(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
