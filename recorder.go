package main

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// recordUntilRelease starts recording audio from the default microphone using sox,
// waits for the hotkey release signal, then stops recording and returns the WAV file path.
func recordUntilRelease() (string, error) {
	// Create temp file path using os.CreateTemp for safety
	tempDir := os.TempDir()
	f, err := os.CreateTemp(tempDir, "voice-dictate-*.wav")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	audioPath := f.Name()
	f.Close() // sox will create the file

	// Start sox process via exec.Command
	cmd := exec.Command(
		"sox",
		"-d",
		"-r",
		"16000",
		"-c",
		"1",
		"-b",
		"16",
		"-e",
		"signed-integer",
		audioPath,
	)
	if err := cmd.Start(); err != nil {
		os.Remove(audioPath)
		return "", fmt.Errorf("failed to start sox recording: %w\n"+
			"Make sure sox is installed: brew install sox", err)
	}

	// Use a channel to coordinate process cleanup
	done := make(chan struct{})
	defer func() {
		// Signal the goroutine to stop if it hasn't already
		select {
		case <-done:
		default:
			close(done)
			cmd.Process.Signal(os.Interrupt)
			cmd.Wait() // reap the process
		}
	}()

	// Safely wait for release signal
	sig := getReleaseSignal()
	if sig == nil {
		return "", fmt.Errorf("release signal not initialized")
	}
	<-sig

	// Stop sox by sending SIGINT
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		return "", fmt.Errorf("failed to stop sox recording: %w", err)
	}
	close(done)

	// Wait for process to exit
	if err := cmd.Wait(); err != nil {
		// SIGINT causes a non-zero exit, which is expected
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 1 {
				return "", fmt.Errorf("sox exited unexpectedly: %w", err)
			}
		} else {
			return "", fmt.Errorf("sox process error: %w", err)
		}
	}

	// Verify file was created
	info, err := os.Stat(audioPath)
	if err != nil {
		return "", fmt.Errorf("recording file was not created: %w", err)
	}
	if info.Size() < 1000 {
		return "", fmt.Errorf(
			"recording file is too small (%d bytes), no audio captured",
			info.Size(),
		)
	}

	return audioPath, nil
}

// Global state for coordinating recording and hotkey
var (
	releaseSignal chan struct{}
	releaseMu     sync.Mutex
)

// initReleaseSignal creates a new release signal channel.
func initReleaseSignal() {
	releaseMu.Lock()
	defer releaseMu.Unlock()
	releaseSignal = make(chan struct{})
}

// signalRelease closes the current release signal channel and creates a new one.
func signalRelease() {
	releaseMu.Lock()
	defer releaseMu.Unlock()
	if releaseSignal == nil {
		return // guard against nil channel close
	}
	close(releaseSignal)
	releaseSignal = make(chan struct{})
}

// getReleaseSignal returns the current release signal channel.
func getReleaseSignal() chan struct{} {
	releaseMu.Lock()
	defer releaseMu.Unlock()
	return releaseSignal
}
