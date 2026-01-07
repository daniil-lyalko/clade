package context

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TodoItem represents a TODO comment found in code
type TodoItem struct {
	File    string
	Line    int
	Content string
}

// Common file extensions to scan for TODOs
var scanExtensions = map[string]bool{
	".go":   true,
	".js":   true,
	".ts":   true,
	".tsx":  true,
	".jsx":  true,
	".py":   true,
	".rb":   true,
	".java": true,
	".c":    true,
	".cpp":  true,
	".h":    true,
	".rs":   true,
	".php":  true,
	".sh":   true,
	".yaml": true,
	".yml":  true,
}

// Directories to skip
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	".git":         true,
	"dist":         true,
	"build":        true,
	"__pycache__":  true,
	".next":        true,
	"target":       true,
}

// FindTodos scans a directory for TODO comments in source files
func FindTodos(dir string, maxResults int) ([]TodoItem, error) {
	var todos []TodoItem
	todoPattern := regexp.MustCompile(`(?i)\b(TODO|FIXME|HACK|XXX|BUG)\b[:\s]*(.*)`)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip directories
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if we've found enough
		if len(todos) >= maxResults {
			return filepath.SkipAll
		}

		// Check file extension
		ext := strings.ToLower(filepath.Ext(path))
		if !scanExtensions[ext] {
			return nil
		}

		// Scan file for TODOs
		fileTodos, err := scanFileForTodos(path, todoPattern)
		if err != nil {
			return nil // Skip files we can't read
		}

		// Make paths relative
		relPath, _ := filepath.Rel(dir, path)
		for i := range fileTodos {
			fileTodos[i].File = relPath
		}

		todos = append(todos, fileTodos...)
		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return nil, err
	}

	// Limit results
	if len(todos) > maxResults {
		todos = todos[:maxResults]
	}

	return todos, nil
}

func scanFileForTodos(path string, pattern *regexp.Regexp) ([]TodoItem, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var todos []TodoItem
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		matches := pattern.FindStringSubmatch(line)
		if len(matches) >= 3 {
			content := strings.TrimSpace(matches[0])
			todos = append(todos, TodoItem{
				File:    path,
				Line:    lineNum,
				Content: content,
			})
		}
	}

	return todos, scanner.Err()
}
