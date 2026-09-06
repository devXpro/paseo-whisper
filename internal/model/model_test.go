package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devXpro/paseo-whisper/internal/discover"
)

func TestBestDiscoveredPrefersTurbo(t *testing.T) {
	models := []discover.Model{
		{Name: "large-v3", Usable: true, SizeMB: 2950},
		{Name: "large-v3-turbo", Usable: true, SizeMB: 1550},
	}
	got, ok := BestDiscovered(models)
	if !ok {
		t.Fatal("a usable model should have been picked")
	}
	if got.Name != "large-v3-turbo" {
		t.Fatalf("turbo must win even when a larger model is present, got %s", got.Name)
	}
}

func TestBestDiscoveredSkipsUnusable(t *testing.T) {
	models := []discover.Model{
		{Name: "coreml-bundle", Usable: false, SizeMB: 3000},
		{Name: "large-v3", Usable: true, SizeMB: 2950},
	}
	got, ok := BestDiscovered(models)
	if !ok || got.Name != "large-v3" {
		t.Fatalf("unusable models must be skipped, got %+v", got)
	}
}

func TestBestDiscoveredEmpty(t *testing.T) {
	if _, ok := BestDiscovered(nil); ok {
		t.Fatal("nothing to pick from should report false")
	}
}

func TestValidateRejectsCoreMLDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "openai_whisper-large-v3", "AudioEncoder.mlmodelc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := Validate(dir)
	if err == nil {
		t.Fatal("a CoreML bundle must be rejected")
	}
}

func TestValidateRejectsTinyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ggml-fake.bin")
	if err := os.WriteFile(path, []byte("not a model"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Validate(path); err == nil {
		t.Fatal("a stub file must be rejected")
	}
}

func TestAdoptHardLinkSharesInode(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "ggml-src.bin")
	if err := os.WriteFile(src, make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}

	dstDir := filepath.Join(tmp, "models")
	got, err := Adopt(src, dstDir, LinkHard)
	if err != nil {
		t.Fatal(err)
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	dstInfo, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(srcInfo, dstInfo) {
		t.Fatal("hard link should point at the same inode")
	}
}

func TestAdoptNoneKeepsOriginalPath(t *testing.T) {
	got, err := Adopt("/some/model.bin", "/unused", LinkNone)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/some/model.bin" {
		t.Fatalf("LinkNone must not move the file, got %s", got)
	}
}
