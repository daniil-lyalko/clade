package main

import (
	"os"

	"github.com/daniil-lyalko/pacer/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
