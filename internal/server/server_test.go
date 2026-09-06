package server

import (
	"mime"
	"mime/multipart"
	"strings"
	"testing"
)

func TestBuildUpstreamFormCarriesFileAndHints(t *testing.T) {
	body, contentType, err := buildUpstreamForm(strings.NewReader("RIFFfake"), "clip.wav", "ru", "Docker, Postgres")
	if err != nil {
		t.Fatal(err)
	}

	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(body, params["boundary"])
	form, err := reader.ReadForm(1 << 20)
	if err != nil {
		t.Fatal(err)
	}

	if len(form.File["file"]) != 1 {
		t.Fatal("the audio file must be forwarded")
	}
	if got := form.Value["language"]; len(got) != 1 || got[0] != "ru" {
		t.Fatalf("language not forwarded: %v", got)
	}
	if got := form.Value["prompt"]; len(got) != 1 || got[0] != "Docker, Postgres" {
		t.Fatalf("prompt not forwarded: %v", got)
	}
	if got := form.Value["response_format"]; len(got) != 1 || got[0] != "json" {
		t.Fatalf("response_format must be json, got %v", got)
	}
}

func TestBuildUpstreamFormOmitsEmptyHints(t *testing.T) {
	body, contentType, err := buildUpstreamForm(strings.NewReader("RIFFfake"), "clip.wav", "", "")
	if err != nil {
		t.Fatal(err)
	}

	_, params, _ := mime.ParseMediaType(contentType)
	form, err := multipart.NewReader(body, params["boundary"]).ReadForm(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := form.Value["language"]; ok {
		t.Fatal("an empty language must not be sent")
	}
	if _, ok := form.Value["prompt"]; ok {
		t.Fatal("an empty prompt must not be sent")
	}
}
