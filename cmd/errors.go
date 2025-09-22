package cmd

import (
	"fmt"
	"os"
)

// handleError prints error message and exits with code 1
func handleError(msg string, err error) {
	fmt.Fprintf(os.Stderr, "❌ %s: %v\n", msg, err)
	os.Exit(1)
}
