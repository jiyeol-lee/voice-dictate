package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// typeText types the given text into the active window using AppleScript.
// For long text, it chunks the input to avoid AppleScript limitations.
func typeText(text string) error {
	if text == "" {
		return nil
	}

	// Chunk text if it's too long (AppleScript has string length limits)
	const maxChunkSize = 1000
	chunks := chunkString(text, maxChunkSize)

	for _, chunk := range chunks {
		escaped := escapeAppleScript(chunk)
		script := fmt.Sprintf(`tell application "System Events" to keystroke "%s"`, escaped)

		cmd := exec.Command("osascript", "-e", script)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("osascript failed: %w", err)
		}
	}

	return nil
}

// showNotification displays a macOS notification with the given message.
func showNotification(message string) {
	escaped := escapeAppleScript(message)
	script := fmt.Sprintf(`display notification "%s" with title "Voice Dictate"`, escaped)
	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		// If notification fails, print to terminal but don't fail
		fmt.Printf("Notification: %s\n", message)
	}
}

// escapeAppleScript escapes special characters for use in AppleScript strings.
func escapeAppleScript(s string) string {
	// Escape backslash first to avoid double-escaping
	s = strings.ReplaceAll(s, `\`, `\\`)
	// Escape double quotes
	s = strings.ReplaceAll(s, `"`, `\"`)
	// Escape ampersand (AppleScript concatenation operator)
	s = strings.ReplaceAll(s, "&", `\\&`)
	// Escape backticks (shell interpretation)
	s = strings.ReplaceAll(s, "`", `\\`+"`")
	// Escape dollar signs (variable expansion)
	s = strings.ReplaceAll(s, "$", `\$`)
	// Escape null bytes
	s = strings.ReplaceAll(s, "\x00", "")
	// Escape newlines
	s = strings.ReplaceAll(s, "\n", `\" & (ASCII character 10) & \"`)
	// Escape carriage returns
	s = strings.ReplaceAll(s, "\r", `\" & (ASCII character 13) & \"`)
	// Escape tabs
	s = strings.ReplaceAll(s, "\t", `\" & (ASCII character 9) & \"`)
	return s
}

// chunkString splits a string into chunks of at most n runes.
// It tries to split on word boundaries to avoid breaking words.
func chunkString(s string, n int) []string {
	runes := []rune(s)
	if len(runes) <= n {
		return []string{s}
	}

	var chunks []string
	for len(runes) > n {
		// Find the last space within the chunk limit
		splitIdx := n
		if idx := strings.LastIndex(string(runes[:n]), " "); idx > n/2 {
			splitIdx = idx
		}
		chunks = append(chunks, string(runes[:splitIdx]))
		runes = runes[splitIdx:]
		// Trim leading space from remaining string
		for len(runes) > 0 && runes[0] == ' ' {
			runes = runes[1:]
		}
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}
