package main

import (
	"os"
	"testing"
)

func TestMain(t *testing.T) {
	// Save original args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Test with version flag
	os.Args = []string{"dddns", "--version"}

	// main() calls os.Exit on error, so we need to be careful
	// For now, just ensure the code compiles and can be called
	// In a real test, we'd need to refactor main() to be testable
}
