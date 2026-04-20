package main

import (
	"fmt"
	"os"
	"strings"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

// OfflineRecognizer wraps the sherpa-onnx offline recognizer.
type OfflineRecognizer struct {
	recognizer *sherpa.OfflineRecognizer
}

// loadModel creates and configures the sherpa-onnx offline recognizer.
func loadModel(paths *ModelPaths, verbose bool) (*OfflineRecognizer, error) {
	config := sherpa.OfflineRecognizerConfig{}

	// Configure model paths for nemo_transducer (Parakeet TDT)
	config.ModelConfig.Transducer.Encoder = paths.Encoder
	config.ModelConfig.Transducer.Decoder = paths.Decoder
	config.ModelConfig.Transducer.Joiner = paths.Joiner
	config.ModelConfig.Tokens = paths.Tokens

	// Model type for faster loading
	config.ModelConfig.ModelType = "nemo_transducer"

	// Use 4 threads for Apple Silicon performance
	config.ModelConfig.NumThreads = 4

	// Provider
	config.ModelConfig.Provider = "cpu"

	// Debug mode
	if verbose {
		config.ModelConfig.Debug = 1
	}

	// Feature config
	config.FeatConfig.SampleRate = 16000
	config.FeatConfig.FeatureDim = 80

	// Decoding method
	config.DecodingMethod = "greedy_search"

	// Create recognizer
	recognizer := sherpa.NewOfflineRecognizer(&config)
	if recognizer == nil {
		return nil, fmt.Errorf("failed to create offline recognizer")
	}

	return &OfflineRecognizer{recognizer: recognizer}, nil
}

// Close releases the recognizer resources.
func (r *OfflineRecognizer) Close() {
	if r.recognizer != nil {
		sherpa.DeleteOfflineRecognizer(r.recognizer)
	}
}

// transcribe processes an audio file and returns the transcribed text.
func transcribe(
	recognizer *OfflineRecognizer,
	audioPath string,
	language string,
	verbose bool,
) (string, error) {
	if verbose && language != "" {
		fmt.Fprintf(
			os.Stderr,
			"Transcription language: %s (auto-detect if not supported by model)\n",
			language,
		)
	}
	if recognizer == nil || recognizer.recognizer == nil {
		return "", fmt.Errorf("recognizer not initialized")
	}

	// Read the wave file
	wave := sherpa.ReadWave(audioPath)
	if wave == nil {
		return "", fmt.Errorf("failed to read audio file: %s", audioPath)
	}

	// Create a new stream for this transcription
	stream := sherpa.NewOfflineStream(recognizer.recognizer)
	if stream == nil {
		return "", fmt.Errorf("failed to create offline stream")
	}
	defer sherpa.DeleteOfflineStream(stream)

	// Feed audio samples to the stream
	stream.AcceptWaveform(wave.SampleRate, wave.Samples)

	// Decode the stream
	recognizer.recognizer.Decode(stream)

	// Get the result
	result := stream.GetResult()
	if result == nil {
		return "", fmt.Errorf("transcription returned nil result")
	}

	text := strings.ToLower(result.Text)

	if verbose {
		fmt.Fprintf(os.Stderr, "Raw transcription result: %q\n", text)
	}

	return text, nil
}
