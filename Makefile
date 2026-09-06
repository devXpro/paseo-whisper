BINARY      := paseo-whisper
BIN_DIR     := bin
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -ldflags "-s -w -X main.version=$(VERSION)"
PLIST       := com.devxpro.paseo-whisper.plist
LAUNCH_DIR  := $(HOME)/Library/LaunchAgents
INSTALL_DIR := $(HOME)/.local/bin

.DEFAULT_GOAL := help
.PHONY: help build run setup doctor install uninstall status logs test testdata fmt vet clean \
        use-whisper use-parakeet paseo-status paseo-restart

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

paseo-status: build ## Show which engine Paseo dictation uses
	./$(BIN_DIR)/$(BINARY) paseo status

paseo-restart: build ## Restart the Paseo daemon and warm agents
	./$(BIN_DIR)/$(BINARY) paseo restart

install: build ## Install the binary and start it at login via launchd
	@mkdir -p $(INSTALL_DIR) $(LAUNCH_DIR)
	cp $(BIN_DIR)/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@sed -e 's|@BINARY@|$(INSTALL_DIR)/$(BINARY)|g' \
	     -e 's|@LOGDIR@|$(HOME)/.config/paseo-whisper|g' \
	     launchd/$(PLIST).in > $(LAUNCH_DIR)/$(PLIST)
	@mkdir -p $(HOME)/.config/paseo-whisper
	launchctl unload $(LAUNCH_DIR)/$(PLIST) 2>/dev/null || true
	launchctl load -w $(LAUNCH_DIR)/$(PLIST)
	@echo "installed and loaded. check: make status"

uninstall: ## Stop the service and remove it
	launchctl unload $(LAUNCH_DIR)/$(PLIST) 2>/dev/null || true
	rm -f $(LAUNCH_DIR)/$(PLIST) $(INSTALL_DIR)/$(BINARY)
	@echo "removed. config and models remain in ~/.config/paseo-whisper"

status: ## Show whether the service is running and healthy
	@launchctl list | grep -q paseo-whisper && echo "launchd: loaded" || echo "launchd: not loaded"
	@curl -fsS http://127.0.0.1:8099/health 2>/dev/null || echo "health:  not responding on :8099"

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
