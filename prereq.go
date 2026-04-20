package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// checkPrerequisites verifies that all required tools are installed.
func checkPrerequisites() error {
	// Check if sox is available
	if _, err := exec.LookPath("sox"); err != nil {
		return fmt.Errorf("sox is not installed\n" +
			"Install it with: brew install sox\n" +
			"\n" +
			"sox is used to record audio from your microphone.")
	}

	// Check temp directory writability
	tempDir := os.TempDir()
	testFile := filepath.Join(tempDir, "voice-dictate-temp-check")
	if err := os.WriteFile(testFile, []byte{}, 0o644); err != nil {
		return fmt.Errorf("temp directory is not writable: %s\n"+
			"Check permissions on %s", tempDir, tempDir)
	}
	os.Remove(testFile)

	return nil
}
