.PHONY: build install clean test

BINARY=clade
INSTALL_DIR=$(HOME)/.local/bin

build:
	go build -o $(BINARY) ./cmd/clade

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed to $(INSTALL_DIR)/$(BINARY)"
	@echo "Make sure $(INSTALL_DIR) is in your PATH"

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
