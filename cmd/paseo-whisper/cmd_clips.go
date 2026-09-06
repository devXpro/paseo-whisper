package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/devXpro/paseo-whisper/internal/config"
	"github.com/devXpro/paseo-whisper/internal/server"
)

// clipsCommand implements `paseo-whisper clips [list|play|path]`.
func clipsCommand(sub string, rest []string) error {
	clips, err := server.LoadClips(config.ClipsDir())
	if err != nil {
		return err
	}
	if len(clips) == 0 {
		fmt.Println("\n  No saved clips. Start the server with --save-clips and dictate something.")
		fmt.Println()
		return nil
	}

	switch sub {
	case "", "list":
		return clipsList(clips)
	case "path":
		return clipsPath(clips, argOr(rest, "1"))
	case "play":
		return clipsPlay(clips, argOr(rest, "1"))
	default:
		return fmt.Errorf("unknown clips command %q (try: list, play, path)", sub)
	}
}

func clipsList(clips []server.Clip) error {
	fmt.Printf("\n  %d saved clips, newest first (%s)\n\n", len(clips), config.ClipsDir())
	for i, c := range clips {
		if i >= 20 {
			fmt.Printf("  ... and %d older\n", len(clips)-20)
			break
		}
		fmt.Printf("  %2d) %s  %5dms  %4d KB\n", i+1,
			c.Recorded.Format("15:04:05"), c.Millis, c.Bytes>>10)
		fmt.Printf("      %s\n", truncate(c.Text, 100))
	}
	fmt.Println("\n  replay one:  paseo-whisper clips play 1")
	fmt.Println()
	return nil
}

// clipsPath prints the audio file path, so it can be piped into other tools.
func clipsPath(clips []server.Clip, arg string) error {
	clip, err := pick(clips, arg)
	if err != nil {
		return err
	}
	fmt.Println(server.AudioPath(config.ClipsDir(), clip))
	return nil
}

func clipsPlay(clips []server.Clip, arg string) error {
	clip, err := pick(clips, arg)
	if err != nil {
		return err
	}
	path := server.AudioPath(config.ClipsDir(), clip)
	fmt.Printf("\n  %s\n  transcribed as: %s\n\n", path, clip.Text)

	cmd := exec.Command("afplay", path)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func pick(clips []server.Clip, arg string) (server.Clip, error) {
	index := 1
	if _, err := fmt.Sscanf(arg, "%d", &index); err != nil {
		return server.Clip{}, fmt.Errorf("expected a clip number, got %q", arg)
	}
	if index < 1 || index > len(clips) {
		return server.Clip{}, fmt.Errorf("no clip %d; there are %d", index, len(clips))
	}
	return clips[index-1], nil
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "..."
}
