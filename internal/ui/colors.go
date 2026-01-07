package ui

import (
	"fmt"

	"github.com/fatih/color"
)

var (
	// Colors
	Green   = color.New(color.FgGreen).SprintFunc()
	Yellow  = color.New(color.FgYellow).SprintFunc()
	Red     = color.New(color.FgRed).SprintFunc()
	Cyan    = color.New(color.FgCyan).SprintFunc()
	Magenta = color.New(color.FgMagenta).SprintFunc()
	Bold    = color.New(color.Bold).SprintFunc()
	Dim     = color.New(color.Faint).SprintFunc()
)

// Success prints a success message
func Success(format string, args ...interface{}) {
	fmt.Printf("%s %s\n", Green("✓"), fmt.Sprintf(format, args...))
}

// Info prints an info message
func Info(format string, args ...interface{}) {
	fmt.Printf("%s %s\n", Cyan("→"), fmt.Sprintf(format, args...))
}

// Warn prints a warning message
func Warn(format string, args ...interface{}) {
	fmt.Printf("%s %s\n", Yellow("⚠"), fmt.Sprintf(format, args...))
}

// Error prints an error message
func Error(format string, args ...interface{}) {
	fmt.Printf("%s %s\n", Red("✗"), fmt.Sprintf(format, args...))
}

// Header prints a bold header
func Header(format string, args ...interface{}) {
	fmt.Printf("\n%s\n", Bold(fmt.Sprintf(format, args...)))
}

// Detail prints an indented detail line
func Detail(format string, args ...interface{}) {
	fmt.Printf("  %s\n", fmt.Sprintf(format, args...))
}

// KeyValue prints a key-value pair
func KeyValue(key, value string) {
	fmt.Printf("  %s: %s\n", Dim(key), value)
}
