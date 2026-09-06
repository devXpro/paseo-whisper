package discover

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDirFindsGGMLAndSkipsStubs(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "ggml-large-v3-turbo.bin")
	if err := os.WriteFile(real, make([]byte, 11<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(root, "ggml-partial.bin")
	if err := os.WriteFile(stub, []byte("tiny"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := scanDir(root, "test")
	if len(got) != 1 {
		t.Fatalf("expected exactly one model, got %d", len(got))
	}
	if got[0].Name != "large-v3-turbo" {
		t.Fatalf("unexpected name %q", got[0].Name)
	}
	if !got[0].Usable {
		t.Fatal("a ggml file should be usable")
	}
}

func TestScanDirIgnoresSmallCoreMLBlobs(t *testing.T) {
	root := t.TempDir()
	blob := filepath.Join(root, "W8A16", "weights.mlmodelc")
	if err := os.MkdirAll(blob, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blob, "weight.bin"), make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := scanDir(root, "test"); len(got) != 0 {
		t.Fatalf("auxiliary CoreML blobs must be ignored, got %d", len(got))
	}
}

func TestSortPutsUsableFirst(t *testing.T) {
	models := []Model{
		{Name: "coreml", Usable: false, SizeMB: 3000},
		{Name: "small", Usable: true, SizeMB: 500},
		{Name: "large", Usable: true, SizeMB: 1500},
	}
	sortModels(models)

	if models[0].Name != "large" {
		t.Fatalf("largest usable model should rank first, got %s", models[0].Name)
	}
	if models[2].Usable {
		t.Fatal("unusable models must sink to the bottom")
	}
}
