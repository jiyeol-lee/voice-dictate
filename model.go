package main

import (
	"archive/tar"
	"compress/bzip2"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	modelURL      = "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8.tar.bz2"
	modelFileName = "sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8.tar.bz2"
)

// ModelPaths holds the paths to all required model files.
type ModelPaths struct {
	Encoder string
	Decoder string
	Joiner  string
	Tokens  string
}

// requiredFiles lists the expected model files with their minimum sizes in bytes.
var requiredFiles = map[string]int64{
	"encoder.int8.onnx": 100 * 1024 * 1024, // At least 100MB
	"decoder.int8.onnx": 1 * 1024 * 1024,   // At least 1MB
	"joiner.int8.onnx":  1 * 1024 * 1024,   // At least 1MB
	"tokens.txt":        1024,              // At least 1KB
}

// ensureModel checks if model files exist and are valid, downloads if missing.
func ensureModel(modelDir string, verbose bool) (*ModelPaths, error) {
	// Create model directory if it doesn't exist
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create model directory: %w", err)
	}

	// Check if all required files exist and are valid
	paths := &ModelPaths{
		Encoder: filepath.Join(modelDir, "encoder.int8.onnx"),
		Decoder: filepath.Join(modelDir, "decoder.int8.onnx"),
		Joiner:  filepath.Join(modelDir, "joiner.int8.onnx"),
		Tokens:  filepath.Join(modelDir, "tokens.txt"),
	}

	allValid := true
	for fileName, minSize := range requiredFiles {
		filePath := filepath.Join(modelDir, fileName)
		info, err := os.Stat(filePath)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Model file missing: %s\n", fileName)
			}
			allValid = false
			break
		}
		if info.Size() < minSize {
			if verbose {
				fmt.Fprintf(
					os.Stderr,
					"Model file too small: %s (%d bytes, expected at least %d)\n",
					fileName,
					info.Size(),
					minSize,
				)
			}
			allValid = false
			break
		}
	}

	if allValid {
		if verbose {
			fmt.Fprintln(os.Stderr, "Model files verified, using cached version")
		}
		return paths, nil
	}

	// Download model
	fmt.Println("Model not found. Downloading model (~640MB, this may take a while)...")
	fmt.Println("This only happens on the first run.")
	fmt.Println()

	if err := downloadAndExtract(modelDir, verbose); err != nil {
		return nil, fmt.Errorf("failed to download model: %w", err)
	}

	// Verify after download
	for fileName, minSize := range requiredFiles {
		filePath := filepath.Join(modelDir, fileName)
		info, err := os.Stat(filePath)
		if err != nil {
			return nil, fmt.Errorf("model file still missing after download: %s", fileName)
		}
		if info.Size() < minSize {
			return nil, fmt.Errorf(
				"model file too small after download: %s (%d bytes)",
				fileName,
				info.Size(),
			)
		}
	}

	fmt.Println("Model downloaded and verified successfully.")
	fmt.Println()

	return paths, nil
}

// downloadAndExtract downloads the model archive and extracts it.
func downloadAndExtract(modelDir string, verbose bool) error {
	// Create a temporary directory for extraction
	tempDir, err := os.MkdirTemp("", "voice-dictate-model-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Download the archive
	archivePath := filepath.Join(tempDir, modelFileName)
	if err := downloadFile(modelURL, archivePath, verbose); err != nil {
		return err
	}

	// Extract the archive
	if err := extractTarBz2(archivePath, modelDir, verbose); err != nil {
		return fmt.Errorf("failed to extract model: %w", err)
	}

	return nil
}

// downloadFile downloads a file from URL to destPath with progress reporting.
func downloadFile(url string, destPath string, verbose bool) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf(
			"failed to start download: %w\nCheck your internet connection and try again.",
			err,
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"download failed with status: %s\nThe model server may be temporarily unavailable. Try again later.",
			resp.Status,
		)
	}

	// Create destination file
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Get content length for progress
	contentLength := resp.ContentLength
	if contentLength > 0 && verbose {
		fmt.Fprintf(os.Stderr, "Downloading: %d bytes\n", contentLength)
	}

	// Copy with progress
	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("download interrupted: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Downloaded %d bytes\n", written)
	}

	return nil
}

// extractTarBz2 extracts a .tar.bz2 archive to destDir.
func extractTarBz2(archivePath string, destDir string, verbose bool) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer f.Close()

	bz2Reader := bzip2.NewReader(f)
	tarReader := tar.NewReader(bz2Reader)

	extracted := 0
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read archive: %w", err)
		}

		// Only extract the model files we need
		baseName := filepath.Base(header.Name)
		if _, ok := requiredFiles[baseName]; !ok {
			continue
		}

		destPath := filepath.Join(destDir, baseName)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			const maxFileSize = 1 << 30 // 1GB
			if header.Size > maxFileSize {
				return fmt.Errorf("file too large in archive: %s (%d bytes)", baseName, header.Size)
			}
			outFile, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			limitedReader := io.LimitReader(tarReader, maxFileSize)
			if _, err := io.Copy(outFile, limitedReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file: %w", err)
			}
			outFile.Close()

			extracted++
			if verbose {
				fmt.Fprintf(os.Stderr, "Extracted: %s\n", baseName)
			}
		}
	}

	if extracted == 0 {
		return fmt.Errorf("no model files found in archive")
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Extracted %d model files\n", extracted)
	}

	return nil
}

var lockFile *os.File

// acquireLock ensures only one instance of voice-dictate runs at a time.
func acquireLock(lockPath string) error {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return fmt.Errorf("voice-dictate is already running.\n"+
			"If this is incorrect, remove the lock file: %s", lockPath)
	}

	lockFile = f
	return nil
}

// releaseLock removes the lock file.
func releaseLock(lockPath string) {
	if lockFile != nil {
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		lockFile = nil
	}
	os.Remove(lockPath)
}
