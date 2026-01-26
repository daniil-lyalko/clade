.PHONY: build install reinstall clean test

BINARY=clade
INSTALL_DIR=$(HOME)/.local/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X github.com/daniil-lyalko/clade/internal/cmd.Version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/clade

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
	go test ./...

# Development helpers
run:
	go run ./cmd/clade $(ARGS)

fmt:
	go fmt ./...

lint:
	golangci-lint run

# Shell completion setup
install-completions:
	@echo "Installing zsh completions..."
	@if command -v brew >/dev/null 2>&1; then \
		$(INSTALL_DIR)/$(BINARY) completion zsh > $$(brew --prefix)/share/zsh/site-functions/_clade && \
		echo "✓ Installed to Homebrew site-functions" && \
		echo "  Reload shell: exec zsh"; \
	else \
		mkdir -p ~/.zsh/completions && \
		$(INSTALL_DIR)/$(BINARY) completion zsh > ~/.zsh/completions/_clade && \
		echo "✓ Installed to ~/.zsh/completions" && \
		echo "  Add to ~/.zshrc if not present: fpath=(~/.zsh/completions \$$fpath)" && \
		echo "  Reload shell: exec zsh"; \
	fi
