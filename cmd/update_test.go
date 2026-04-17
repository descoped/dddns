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

// TestWriteCachedIP_NestedPath verifies directory creation works for multi-level paths.
func TestWriteCachedIP_NestedPath(t *testing.T) {
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "nested", "deeper", "cache.txt")

	if err := writeCachedIP(cacheFile, "1.2.3.4"); err != nil {
		t.Fatalf("writeCachedIP failed on nested path: %v", err)
	}
	if _, err := os.Stat(cacheFile); err != nil {
		t.Errorf("cache file not created at %s: %v", cacheFile, err)
	}
}

// TestWriteCachedIP_RelativePath verifies a path with no separator succeeds.
// Previously, strings.LastIndex(path, "/") returned -1, making the slice
// path[:-1] panic at runtime.
func TestWriteCachedIP_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	if err := writeCachedIP("cache.txt", "1.2.3.4"); err != nil {
		t.Fatalf("writeCachedIP failed on bare filename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "cache.txt")); err != nil {
		t.Errorf("cache file not created: %v", err)
	}
}
