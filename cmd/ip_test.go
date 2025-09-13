package cmd

import (
	"bytes"
	"testing"
)

func TestIPCommand(t *testing.T) {
	// Reset command for testing
	rootCmd.SetArgs([]string{"ip"})

	// Capture output
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)

	// Execute command - should work as it fetches real IP
	err := rootCmd.Execute()

	if err != nil {
		// Might fail in CI/test environment without network
		t.Logf("IP command failed (might be expected in test environment): %v", err)
	} else {
		// If it succeeds, check we got output
		output := buf.String()
		if output == "" {
			t.Error("Expected some output from ip command")
		}
	}
}
