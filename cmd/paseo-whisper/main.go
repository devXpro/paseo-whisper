// Command paseo-whisper serves an OpenAI-compatible transcription API backed
// by a local whisper.cpp engine, so Paseo dictation can run entirely offline.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/devXpro/paseo-whisper/internal/config"
	"github.com/devXpro/paseo-whisper/internal/discover"
	"github.com/devXpro/paseo-whisper/internal/model"
	"github.com/devXpro/paseo-whisper/internal/server"
	"github.com/devXpro/paseo-whisper/internal/whisper"
	"github.com/devXpro/paseo-whisper/internal/wizard"
)

// version is overridden at build time via -ldflags.
var version = "dev"

type flags struct {
	modelPath  string
	source     string
	link       string
	promptFile string
	engine     string
	language   string
	port       int
	threads    int
	yes        bool
	reset      bool
	restart    bool
	saveClips  bool
}

func main() {
	if err := run(); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	command, sub := "serve", ""
	args := os.Args[1:]
	if len(args) > 0 && !isFlag(args[0]) {
		command, args = args[0], args[1:]
		// Two-word commands: "model download", "paseo use".
		if (command == "paseo" || command == "clips") && len(args) > 0 && !isFlag(args[0]) {
			sub, args = args[0], args[1:]
		}
	}

	fs := flag.NewFlagSet("paseo-whisper", flag.ExitOnError)
	var f flags
	fs.StringVar(&f.modelPath, "model", "", "path to a ggml model file")
	fs.StringVar(&f.source, "model-source", "", "auto|superwhisper|download|path - where the model file comes from")
	fs.StringVar(&f.link, "link", "", "hardlink|copy|none - how to adopt a model owned by another app")
	fs.StringVar(&f.promptFile, "prompt-file", "", "file with domain vocabulary to bias transcription")
	fs.StringVar(&f.engine, "engine", "", "path to whisper-server (default: autodetect)")
	fs.StringVar(&f.language, "language", "", "spoken language, or auto")
	fs.IntVar(&f.port, "port", 0, "port to listen on")
	fs.IntVar(&f.threads, "threads", 0, "engine worker threads")
	fs.BoolVar(&f.yes, "yes", false, "never prompt; resolve everything from flags and defaults")
	fs.BoolVar(&f.reset, "reset", false, "ignore the saved config and choose again")
	fs.BoolVar(&f.restart, "restart", false, "restart the Paseo daemon after changing its config")
	fs.BoolVar(&f.saveClips, "save-clips", false, "keep each recording and its transcript for later replay")
	fs.Usage = usage(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	switch command {
	case "serve":
		return serve(f, log)
	case "setup":
		return setup(f, log)
	case "doctor":
		return doctor(f)
	case "paseo":
		ctx, cancel := signalContext()
		defer cancel()
		return paseoCommand(ctx, sub, args, f)
	case "clips":
		return clipsCommand(sub, args)
	case "version":
		fmt.Printf("paseo-whisper %s\n", version)
		return nil
	case "help":
		fs.Usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q (try: serve, setup, doctor, paseo, clips, version)", command)
	}
}

func isFlag(s string) bool { return len(s) > 0 && s[0] == '-' }

// argOr returns the first positional argument, or the fallback when the
// command was invoked with flags only.
func argOr(rest []string, fallback string) string {
	if len(rest) > 0 && !isFlag(rest[0]) {
		return rest[0]
	}
	return fallback
}

func usage(fs *flag.FlagSet) func() {
	return func() {
		fmt.Fprint(os.Stderr, `paseo-whisper - local Whisper transcription for Paseo dictation

Usage:
  paseo-whisper [command] [flags]

Commands:
  serve                    Start the transcription server (default)
  setup                    Choose where the model comes from, save it
  doctor                   Show what is installed and what was found
  paseo status             Show which engine Paseo dictation uses
  paseo use whisper        Point Paseo dictation at this server
  paseo use parakeet       Restore Paseo's built-in engine
  paseo restart            Restart the Paseo daemon and warm agents
  clips list               List saved recordings and their transcripts
  clips play <n>           Play back a saved recording
  clips path <n>           Print the path to a saved recording
  version                  Print the version

Examples:
  paseo-whisper                                  interactive setup, then serve
  paseo-whisper serve --yes                      no prompts, reuse whatever is available
  paseo-whisper serve --model-source download --yes
  paseo-whisper serve --model ~/models/ggml-large-v3.bin --port 8099 --yes

Flags:
`)
		fs.PrintDefaults()
	}
}

// resolve produces a runnable config, asking the user only when it has to.
func resolve(ctx context.Context, f flags, log *slog.Logger) (config.Config, error) {
	cfg, saved := config.Load()
	if f.reset {
		cfg, saved = config.Defaults(), false
	}
	cfg.ApplyEnv()
	applyFlags(&cfg, f)

	if f.saveClips {
		cfg.SaveClips = true
	}
	if f.promptFile != "" {
		prompt, err := config.LoadPromptFile(f.promptFile)
		if err != nil {
			return cfg, fmt.Errorf("read prompt file: %w", err)
		}
		cfg.Prompt = prompt
	}

	engine, err := discover.FindEngine(cfg.EnginePath)
	if err != nil {
		return cfg, err
	}
	cfg.EnginePath = engine.Path

	// An explicit path short-circuits everything else.
	if cfg.ModelPath != "" && !f.reset {
		if err := model.Validate(cfg.ModelPath); err == nil {
			if saved {
				log.Info("using saved configuration", "model", cfg.ModelPath)
			}
			return cfg, nil
		} else if f.modelPath != "" {
			return cfg, fmt.Errorf("model %s: %w", cfg.ModelPath, err)
		}
		cfg.ModelPath = "" // saved model disappeared; fall through and re-resolve
	}

	found := discover.Models()

	if f.yes || !isInteractive() {
		return resolveAutomatically(ctx, cfg, f, found)
	}
	return wizard.Run(ctx, cfg, engine, found)
}

// resolveAutomatically implements the non-interactive path used by launchd and
// by --yes.
func resolveAutomatically(ctx context.Context, cfg config.Config, f flags, found []discover.Model) (config.Config, error) {
	switch cfg.Source {
	case config.SourceMacWhisper:
		return cfg, errors.New("MacWhisper stores CoreML/WhisperKit models, which whisper.cpp cannot load; use --model-source superwhisper or download")

	case config.SourceDownload:
		path, err := model.Download(ctx, config.ModelsDir(), nil)
		if err != nil {
			return cfg, err
		}
		cfg.ModelPath, cfg.ModelName = path, model.Turbo.ID
		return cfg, nil

	case config.SourceSuperWhisper:
		for _, m := range found {
			if m.Usable && m.Owner == "superwhisper" {
				return adopt(cfg, m)
			}
		}
		return cfg, errors.New("no usable superwhisper model found")

	default: // auto
		if best, ok := model.BestDiscovered(found); ok {
			return adopt(cfg, best)
		}
		path, err := model.Download(ctx, config.ModelsDir(), nil)
		if err != nil {
			return cfg, fmt.Errorf("no local model found and download failed: %w", err)
		}
		cfg.ModelPath, cfg.ModelName = path, model.Turbo.ID
		return cfg, nil
	}
}

func adopt(cfg config.Config, m discover.Model) (config.Config, error) {
	path, err := model.Adopt(m.Path, config.ModelsDir(), cfg.LinkMode)
	if err != nil {
		return cfg, err
	}
	cfg.ModelPath, cfg.ModelName = path, m.Name
	return cfg, nil
}

func applyFlags(cfg *config.Config, f flags) {
	if f.modelPath != "" {
		cfg.ModelPath = f.modelPath
		cfg.Source = config.SourcePath
	}
	if f.source != "" {
		cfg.Source = config.Source(f.source)
	}
	if f.link != "" {
		cfg.LinkMode = model.LinkMode(f.link)
	}
	if f.port != 0 {
		cfg.Port = f.port
	}
	if f.threads != 0 {
		cfg.Threads = f.threads
	}
	if f.language != "" {
		cfg.Language = f.language
	}
	if f.engine != "" {
		cfg.EnginePath = f.engine
	}
}

func setup(f flags, log *slog.Logger) error {
	ctx, cancel := signalContext()
	defer cancel()

	f.reset = true
	cfg, err := resolve(ctx, f, log)
	if err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Printf("Saved. Start the server with:\n\n  paseo-whisper serve\n\n")
	return nil
}

func serve(f flags, log *slog.Logger) error {
	ctx, cancel := signalContext()
	defer cancel()

	cfg, err := resolve(ctx, f, log)
	if err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		log.Warn("could not save config", "err", err)
	}

	engineLog, err := os.OpenFile(engineLogPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		log.Warn("engine log unavailable", "err", err)
	}
	defer func() {
		if engineLog != nil {
			engineLog.Close()
		}
	}()

	engine := whisper.New(whisper.Options{
		Binary:    cfg.EnginePath,
		ModelPath: cfg.ModelPath,
		Threads:   cfg.Threads,
		Language:  cfg.Language,
		Prompt:    cfg.Prompt,
		LogWriter: engineLog,
	}, log)

	log.Info("loading model, this takes a few seconds", "model", cfg.ModelPath)
	if err := engine.Start(ctx); err != nil {
		return fmt.Errorf("start engine: %w", err)
	}
	defer engine.Stop()

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	clipsDir := ""
	if cfg.SaveClips {
		clipsDir = config.ClipsDir()
		log.Info("saving clips", "dir", clipsDir)
	}

	api := server.New(server.Options{
		Addr:          addr,
		DefaultPrompt: cfg.Prompt,
		ModelName:     cfg.ModelName,
		ClipsDir:      clipsDir,
	}, engine, log)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	log.Info("ready", "endpoint", "http://"+addr+"/v1/audio/transcriptions")
	fmt.Printf("\n  Point Paseo at:  http://%s/v1\n\n", addr)

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("stopped")
	return nil
}

func doctor(f flags) error {
	fmt.Println()
	engine, err := discover.FindEngine(f.engine)
	if err != nil {
		fmt.Printf("  engine:  NOT FOUND\n           %v\n", err)
	} else {
		fmt.Printf("  engine:  %s [%s]", engine.Path, engine.Source)
		if engine.Version != "" {
			fmt.Printf(" whisper-cpp %s", engine.Version)
		}
		fmt.Println()
	}

	total, perf, eff := whisper.CoreSummary()
	if perf > 0 {
		fmt.Printf("  cpu:     %d cores (%d performance, %d efficiency)\n", total, perf, eff)
	} else {
		fmt.Printf("  cpu:     %d cores\n", total)
	}
	fmt.Printf("  threads: %d (auto)\n", whisper.DefaultThreads())

	fmt.Println()
	found := discover.Models()
	if len(found) == 0 {
		fmt.Println("  models:  none found on this machine")
	} else {
		fmt.Println("  models:")
		for _, m := range found {
			state := "usable"
			if !m.Usable {
				state = "unusable"
			}
			fmt.Printf("    %-24s %-14s %6d MB  %s\n", m.Name, m.Owner, m.SizeMB, state)
			if !m.Usable {
				fmt.Printf("      %s\n", m.Reason)
			}
		}
	}

	fmt.Println()
	if cfg, ok := config.Load(); ok {
		fmt.Printf("  config:  %s\n", cfg.ModelPath)
		threads := cfg.Threads
		note := ""
		if threads <= 0 {
			threads, note = whisper.DefaultThreads(), " (auto)"
		}
		fmt.Printf("           port %d, threads %d%s, language %s\n", cfg.Port, threads, note, cfg.Language)
	} else {
		fmt.Println("  config:  not set up yet (run: paseo-whisper setup)")
	}
	fmt.Println()
	return nil
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func isInteractive() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func engineLogPath() string {
	return config.Dir() + "/engine.log"
}
