package service

import "strings"

import "testing"

func TestRenderPlistCarriesArgs(t *testing.T) {
	plist := renderPlist("/usr/local/bin/paseo-whisper", []string{"serve", "--yes"}, "/tmp/logs")

	for _, want := range []string{
		"<string>" + Label + "</string>",
		"<string>/usr/local/bin/paseo-whisper</string>",
		"<string>serve</string>",
		"<string>--yes</string>",
		"/tmp/logs/service.log",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q", want)
		}
	}
}

func TestRenderPlistEscapesPaths(t *testing.T) {
	plist := renderPlist("/opt/a&b/paseo-whisper", nil, "/tmp")
	if strings.Contains(plist, "a&b") {
		t.Fatal("ampersand must be XML-escaped")
	}
	if !strings.Contains(plist, "a&amp;b") {
		t.Fatal("escaped form missing")
	}
}

func TestBinaryFromPlistRoundTrips(t *testing.T) {
	const binary = "/Users/someone/.local/bin/paseo-whisper"
	got := binaryFromPlist(renderPlist(binary, []string{"serve"}, "/tmp"))
	if got != binary {
		t.Fatalf("got %q, want %q", got, binary)
	}
}
