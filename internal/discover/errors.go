package discover

import "errors"

var (
	// ErrEngineNotFound means no whisper-server binary is available.
	ErrEngineNotFound = errors.New("whisper-server not found; install it with: brew install whisper-cpp")
	// ErrNotExecutable means the configured path exists but cannot be run.
	ErrNotExecutable = errors.New("path exists but is not an executable file")
)
