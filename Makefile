BINARY      := paseo-whisper
BIN_DIR     := bin
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -ldflags "-s -w -X main.version=$(VERSION)"

.DEFAULT_GOAL := help
.PHONY: help build run setup doctor install uninstall status logs test testdata fmt vet clean \
        use-whisper use-parakeet paseo-status paseo-restart clips terms

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary into ./bin
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY) ./cmd/$(BINARY)
	@echo "built $(BIN_DIR)/$(BINARY) ($(VERSION))"

run: build ## Build and start the server (runs setup on first launch)
	./$(BIN_DIR)/$(BINARY) serve

setup: build ## Choose where the model comes from and save the config
	./$(BIN_DIR)/$(BINARY) setup

doctor: build ## Report the engine, models and current config
	./$(BIN_DIR)/$(BINARY) doctor

use-whisper: build ## Point Paseo dictation at this server
	./$(BIN_DIR)/$(BINARY) paseo use whisper

use-parakeet: build ## Restore Paseo's built-in Parakeet engine
	./$(BIN_DIR)/$(BINARY) paseo use parakeet

terms: build ## Mine your chat history for domain vocabulary
	./$(BIN_DIR)/$(BINARY) terms scan

clips: build ## List saved recordings and their transcripts
	./$(BIN_DIR)/$(BINARY) clips list

paseo-status: build ## Show which engine Paseo dictation uses
	./$(BIN_DIR)/$(BINARY) paseo status

paseo-restart: build ## Restart the Paseo daemon and warm agents
	./$(BIN_DIR)/$(BINARY) paseo restart

install: build ## Install as a login agent (starts on login, restarts on crash)
	./$(BIN_DIR)/$(BINARY) install --prompt-file terms.txt --save-clips

uninstall: build ## Stop the login agent and remove it
	./$(BIN_DIR)/$(BINARY) uninstall

status: build ## Show engine, models, service state and config
	./$(BIN_DIR)/$(BINARY) doctor

logs: ## Tail the service and engine logs
	tail -f $(HOME)/.config/paseo-whisper/*.log

test: ## Run the test suite
	go test ./...

testdata: ## Regenerate the Russian sample clip used for manual comparisons
	@mkdir -p testdata
	say -v Milena -o /tmp/paseo-whisper-sample.aiff "$$(cat testdata/ru-jargon.txt)"
	afconvert -f WAVE -d LEI16@16000 -c 1 /tmp/paseo-whisper-sample.aiff testdata/ru-jargon.wav
	@rm -f /tmp/paseo-whisper-sample.aiff
	@echo "wrote testdata/ru-jargon.wav"

fmt: ## Format the code
	gofmt -w .

vet: ## Run go vet
	go vet ./...

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
