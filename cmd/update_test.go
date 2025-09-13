package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadCachedIP(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "cached-ip.txt")

	// Test non-existent file
	ip := readCachedIP(cacheFile)
	if ip != "" {
		t.Errorf("Expected empty string for non-existent file, got %q", ip)
	}

	// Test with cached IP
	testIP := "192.168.1.1"
	err := os.WriteFile(cacheFile, []byte(testIP), 0600)
	if err != nil {
		t.Fatalf("Failed to write cache file: %v", err)
	}

	ip = readCachedIP(cacheFile)
	if ip != testIP {
		t.Errorf("Expected %q, got %q", testIP, ip)
	}
}

func TestWriteCachedIP(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "cached-ip.txt")

	testIP := "10.0.0.1"
	err := writeCachedIP(cacheFile, testIP)
	if err != nil {
		t.Fatalf("Failed to write cached IP: %v", err)
	}

	// Verify file contents
	content, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatalf("Failed to read cache file: %v", err)
	}

	// Cache now includes timestamp, just check it contains the IP
	if !strings.Contains(string(content), testIP) {
		t.Errorf("Expected cache to contain %q, got %q", testIP, string(content))
	}

	// Check permissions
	info, err := os.Stat(cacheFile)
	if err != nil {
		t.Fatalf("Failed to stat cache file: %v", err)
	}

	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("Expected permissions 0600, got %04o", mode)
	}
}
