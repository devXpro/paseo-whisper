// Package server exposes an OpenAI-compatible transcription API backed by a
// local whisper.cpp engine.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/devXpro/paseo-whisper/internal/whisper"
)

// maxUpload caps request bodies. Dictation clips are seconds long, so anything
// larger is a mistake rather than a legitimate request.
const maxUpload = 256 << 20

// Options configures the public HTTP server.
type Options struct {
	Addr          string
	DefaultPrompt string
	ModelName     string
}

// Server translates OpenAI transcription requests into whisper.cpp calls.
type Server struct {
	opts   Options
	engine *whisper.Supervisor
	log    *slog.Logger
	client *http.Client
}

// New builds the HTTP server.
func New(opts Options, engine *whisper.Supervisor, log *slog.Logger) *Server {
	return &Server{
		opts:   opts,
		engine: engine,
		log:    log,
		client: &http.Client{Timeout: 10 * time.Minute},
	}
}

// Handler returns the routes. Both the OpenAI path and whisper.cpp's own
// /inference path are served, so existing clients keep working.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/audio/transcriptions", s.handleTranscribe)
	mux.HandleFunc("POST /inference", s.handleTranscribe)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/models", s.handleModels)
	return s.withLogging(mux)
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path != "/health" { // health is polled constantly; keep logs useful
			s.log.Info("request", "method", r.Method, "path", r.URL.Path, "took", time.Since(start).Round(time.Millisecond))
		}
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	status := "ready"
	code := http.StatusOK
	if !s.engine.Ready() {
		status, code = "loading", http.StatusServiceUnavailable
	}
	writeJSON(w, code, map[string]any{"status": status, "model": s.opts.ModelName})
}

// handleModels answers the OpenAI model listing with the single model we host.
func (s *Server) handleModels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data": []map[string]any{
			{"id": s.opts.ModelName, "object": "model", "owned_by": "paseo-whisper"},
		},
	})
}

func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if !s.engine.Ready() {
		writeError(w, http.StatusServiceUnavailable, "engine is still loading the model")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "expected multipart/form-data: "+err.Error())
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing 'file' field")
		return
	}
	defer file.Close()

	// The client's prompt wins; ours is the fallback vocabulary hint.
	prompt := r.FormValue("prompt")
	if prompt == "" {
		prompt = s.opts.DefaultPrompt
	}

	body, contentType, err := buildUpstreamForm(file, header.Filename, r.FormValue("language"), prompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "build upstream request: "+err.Error())
		return
	}

	text, err := s.transcribe(r.Context(), body, contentType)
	if err != nil {
		s.log.Error("transcription failed", "err", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"text": text})
}

// buildUpstreamForm re-encodes the upload for whisper.cpp, which expects its
// own field names and ignores anything else.
func buildUpstreamForm(file io.Reader, filename, language, prompt string) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, "", err
	}

	fields := map[string]string{"response_format": "json"}
	if language != "" {
		fields["language"] = language
	}
	if prompt != "" {
		fields["prompt"] = prompt
	}
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			return nil, "", err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return &buf, writer.FormDataContentType(), nil
}

// transcribe forwards to the engine and extracts the text from its reply.
func (s *Server) transcribe(ctx context.Context, body *bytes.Buffer, contentType string) (string, error) {
	url := "http://" + s.engine.Addr() + "/inference"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("engine unreachable: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("engine returned %s: %s", resp.Status, strings.TrimSpace(string(payload)))
	}

	var decoded struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return "", fmt.Errorf("unexpected engine response: %s", strings.TrimSpace(string(payload)))
	}
	if decoded.Error != "" {
		return "", fmt.Errorf("engine error: %s", decoded.Error)
	}

	// whisper.cpp pads output with a leading space and a trailing newline.
	return strings.TrimSpace(decoded.Text), nil
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]any{
		"error": map[string]string{"message": message, "type": "paseo_whisper_error"},
	})
}
