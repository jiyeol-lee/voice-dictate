package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
)

const version = "0.1.5"

func main() {
	// Parse flags
	helpFlag := flag.Bool("help", false, "Show usage information")
	versionFlag := flag.Bool("version", false, "Show version")
	hotkeyFlag := flag.String(
		"hotkey",
		"right-cmd",
		"Hotkey to trigger recording (e.g., right-cmd, left-cmd, right-alt)",
	)
	outputOnlyFlag := flag.Bool(
		"output-only",
		false,
		"Print transcription to stdout instead of auto-typing",
	)
	verboseFlag := flag.Bool("verbose", false, "Show debug details")
	languageFlag := flag.String(
		"language",
		"",
		"Language code for transcription (e.g., en, ko). Default: auto-detect",
	)

	flag.Parse()

	// Handle --help
	if *helpFlag {
		printUsage()
		os.Exit(0)
	}

	// Handle --version
	if *versionFlag {
		fmt.Printf("voice-dictate version %s\n", version)
		os.Exit(0)
	}

	// Default command: serve
	serve(*hotkeyFlag, *outputOnlyFlag, *verboseFlag, *languageFlag)
}

func printUsage() {
	fmt.Println(`voice-dictate — Hotkey-triggered voice dictation for macOS

Usage:
  voice-dictate [command] [flags]

Commands:
  serve    Start the dictation daemon (default)

Flags:
  --hotkey <key>     Hotkey to trigger recording (default: right-cmd)
                     Supported: right-cmd, left-cmd, right-alt, left-alt
  --language <code>  Language code for transcription (e.g., en, ko). Default: auto-detect
  --output-only      Print transcription to stdout instead of auto-typing
  --verbose          Show debug details
  --help             Show this help message
  --version          Show version

Examples:
  voice-dictate                    Start daemon with default settings
  voice-dictate serve              Same as above
  voice-dictate --hotkey left-cmd  Use left Command key as trigger
  voice-dictate --output-only      Print text to terminal instead of typing

How it works:
  1. Press and hold the hotkey to start recording
  2. Speak into your microphone
  3. Release the hotkey to stop recording and transcribe
  4. Text is automatically typed into the active window

Requirements:
  - macOS (Apple Silicon recommended)
  - sox: brew install sox
  - Accessibility permissions (for global hotkey and auto-typing)
  - Microphone permissions (for audio recording)`)
}

func hotkeyDisplayName(hotkey string) string {
	switch hotkey {
	case "right-cmd", "right-command":
		return "right ⌘"
	case "left-cmd", "left-command":
		return "left ⌘"
	case "right-alt", "right-option":
		return "right ⌥"
	case "left-alt", "left-option":
		return "left ⌥"
	case "right-shift":
		return "right ⇧"
	case "left-shift":
		return "left ⇧"
	case "right-ctrl", "right-control":
		return "right ⌃"
	case "left-ctrl", "left-control":
		return "left ⌃"
	default:
		return hotkey
	}
}

func serve(hotkey string, outputOnly bool, verbose bool, language string) {
	// Single instance enforcement
	lockPath := filepath.Join(os.TempDir(), "voice-dictate.lock")
	if err := acquireLock(lockPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: voice-dictate is already running.\n")
		fmt.Fprintf(os.Stderr, "If this is incorrect, remove the lock file: %s\n", lockPath)
		os.Exit(1)
	}
	defer releaseLock(lockPath)

	if verbose && language != "" {
		fmt.Fprintf(os.Stderr, "Language: %s\n", language)
	}

	// Check prerequisites
	if err := checkPrerequisites(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Download and verify model
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	modelDir := filepath.Join(homeDir, ".voice-dictate", "models")
	if verbose {
		fmt.Fprintf(os.Stderr, "Model directory: %s\n", modelDir)
	}

	modelPaths, err := ensureModel(modelDir, verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to prepare model: %v\n", err)
		os.Exit(1)
	}

	// Load model
	fmt.Println("Loading model...")
	recognizer, err := loadModel(modelPaths, verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load model: %v\n", err)
		fmt.Fprintf(
			os.Stderr,
			"Try removing the model directory and running again: rm -rf %s\n",
			modelDir,
		)
		os.Exit(1)
	}
	defer recognizer.Close()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Set up hotkey listener
	fmt.Println()
	fmt.Printf("Ready. Press %s to dictate. Ctrl+C to stop.\n", hotkeyDisplayName(hotkey))
	fmt.Println()

	hotkeyEvents, err := setupHotkey(hotkey, verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to register hotkey: %v\n", err)
		fmt.Fprintf(
			os.Stderr,
			"Make sure Accessibility permissions are granted for your terminal.\n",
		)
		fmt.Fprintf(os.Stderr, "Go to: System Settings > Privacy & Security > Accessibility\n")
		os.Exit(1)
	}
	defer cleanupHotkey()

	// Main event loop
	var dictating sync.Mutex
	var wg sync.WaitGroup
	shutdown := make(chan struct{})

	for {
		select {
		case event := <-hotkeyEvents:
			switch event {
			case HotkeyPressed:
				if dictating.TryLock() {
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer dictating.Unlock()
						handleDictationCycle(recognizer, outputOnly, verbose, language)
					}()
				}
			}
		case sig := <-sigChan:
			fmt.Printf("\nReceived %v, shutting down...\n", sig)
			close(shutdown)
			wg.Wait() // wait for any in-flight dictation to finish
			return
		}
	}
}

func handleDictationCycle(
	recognizer *OfflineRecognizer,
	outputOnly bool,
	verbose bool,
	language string,
) {
	fmt.Println("Recording...")
	showNotification("Recording...")

	// Start recording and wait for release
	audioPath, err := recordUntilRelease()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to record audio: %v\n", err)
		fmt.Fprintf(os.Stderr, "Make sure microphone permissions are granted.\n")
		fmt.Fprintf(os.Stderr, "Go to: System Settings > Privacy & Security > Microphone\n")
		showNotification("Recording failed")
		return
	}
	defer os.Remove(audioPath) // Clean up temp file

	fmt.Println("Transcribing...")

	// Transcribe
	text, err := transcribe(recognizer, audioPath, language, verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Transcription failed: %v\n", err)
		showNotification("Transcription failed")
		return
	}

	if text == "" {
		fmt.Println("No speech detected")
		return
	}

	fmt.Printf("Transcribed: %s\n", text)

	if outputOnly {
		fmt.Println(text)
		return
	}

	// Auto-type
	fmt.Println("Typing...")
	if err := typeText(text); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to type text: %v\n", err)
		showNotification("Typing failed")
		return
	}

	// Show notification
	showNotification("Dictation complete")
}
