# Contributing to Clade

Thank you for your interest in contributing to Clade! This document provides guidelines and information for contributors.

## Development Setup

### Prerequisites

- Go 1.22 or later
- Git

### Building from Source

```bash
# Clone the repository
git clone https://github.com/daniil-lyalko/clade.git
cd clade

# Build
go build ./cmd/clade

# Or install directly
go install ./cmd/clade
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/config/...
```

### Linting

We use `golangci-lint` for code quality:

```bash
# Install golangci-lint
brew install golangci-lint  # macOS
# or see https://golangci-lint.run/usage/install/

# Run linter
golangci-lint run
```

## Code Style

- Follow standard Go conventions and formatting (`gofmt`)
- Keep functions focused and small
- Add comments for exported functions and types
- Prefer clarity over cleverness

## Making Changes

### Workflow

1. Fork the repository
2. Create a feature branch from `main`
3. Make your changes
4. Run tests and linting
5. Submit a pull request

### Commit Messages

Write clear, concise commit messages:
- Use imperative mood ("Add feature" not "Added feature")
- Keep the first line under 50 characters
- Add detail in the body if needed

### Pull Requests

- Keep PRs focused on a single change
- Update documentation if needed
- Ensure all tests pass
- Respond to review feedback promptly

## Project Structure

```
pacer/
├── cmd/clade/          # Main entry point
├── internal/
│   ├── cmd/            # Cobra commands
│   ├── config/         # Configuration and state
│   ├── context/        # Context file generation
│   ├── git/            # Git operations
│   └── ui/             # Terminal UI helpers
└── docs/               # Additional documentation
```

## Reporting Issues

When reporting bugs, please include:
- Clade version (`clade --version`)
- Operating system and version
- Steps to reproduce
- Expected vs actual behavior

## Feature Requests

Feature requests are welcome! Please:
- Check existing issues first
- Describe the use case clearly
- Explain why the feature would be useful

## Questions?

Feel free to open an issue for questions about contributing.
