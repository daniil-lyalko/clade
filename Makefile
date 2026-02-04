.PHONY: build install reinstall clean test

BINARY=pacer
INSTALL_DIR=$(HOME)/.local/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X github.com/daniil-lyalko/pacer/internal/cmd.Version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/pacer

install: build
	mkdir -p $(INSTALL_DIR)
	rm -f $(INSTALL_DIR)/$(BINARY)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed to $(INSTALL_DIR)/$(BINARY)"
	@echo "Make sure $(INSTALL_DIR) is in your PATH"

reinstall: clean install
	@echo "Reinstalled from clean slate"

clean:
	rm -f $(BINARY)
	go clean

test:
	@echo "Running tests with coverage..."
	@go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | grep total | awk '{print "Total coverage: " $$3}'

test-verbose:
	go test -race -v ./...

coverage-html:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

coverage-check:
	@go test -coverprofile=coverage.out ./... >/dev/null 2>&1
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	if [ "$${COVERAGE%.*}" -lt 60 ]; then \
		echo "Coverage is below 60%: $${COVERAGE}%"; \
		exit 1; \
	else \
		echo "Coverage is acceptable: $${COVERAGE}%"; \
	fi

# Development helpers
run:
	go run ./cmd/pacer $(ARGS)

fmt:
	go fmt ./...

lint:
	golangci-lint run

# Shell completion setup
install-completions:
	@echo "Installing zsh completions..."
	@if command -v brew >/dev/null 2>&1; then \
		$(INSTALL_DIR)/$(BINARY) completion zsh > $$(brew --prefix)/share/zsh/site-functions/_pacer && \
		echo "✓ Installed to Homebrew site-functions" && \
		echo "  Reload shell: exec zsh"; \
	else \
		mkdir -p ~/.zsh/completions && \
		$(INSTALL_DIR)/$(BINARY) completion zsh > ~/.zsh/completions/_pacer && \
		echo "✓ Installed to ~/.zsh/completions" && \
		echo "  Add to ~/.zshrc if not present: fpath=(~/.zsh/completions \$$fpath)" && \
		echo "  Reload shell: exec zsh"; \
	fi
