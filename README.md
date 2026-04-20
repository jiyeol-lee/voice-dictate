# voice-dictate

Hotkey-triggered voice dictation for macOS. Press a key, speak, release — text appears in your active window. 100% local, no API keys, no network.

## Features

- **Global hotkey** — Press and hold a key (right ⌘ by default) to start recording, release to stop
- **Local transcription** — Uses sherpa-onnx with NVIDIA Parakeet v3 model (INT8 quantized, ~640MB)
- **Auto-type** — Transcribed text is automatically typed into the active window
- **Privacy-first** — No audio or text leaves your machine
- **Free forever** — No API keys, no subscriptions, no usage limits

## Requirements

- macOS (Apple Silicon recommended)
- Go 1.26+
- sox: `brew install sox`
- Accessibility permissions (for global hotkey and auto-typing)
- Microphone permissions (for audio recording)

## Installation

```bash
# Install sox for audio recording
brew install sox

# Build from source
git clone https://github.com/jiyeollee/voice-dictate.git
CGO_ENABLED=1 go build -o voice-dictate .

# Or install directly
CGO_ENABLED=1 go install github.com/jiyeollee/voice-dictate@latest
```

## Usage

```bash
# Start the dictation daemon (default hotkey: right ⌘)
voice-dictate

# Start with a custom hotkey
voice-dictate --hotkey left-ctrl

# Print transcription to stdout instead of auto-typing
voice-dictate --output-only

# Show debug details
voice-dictate --verbose

# Set language (Parakeet v3 auto-detects; flag for future models)
voice-dictate --language en
```

### Workflow

1. Run `voice-dictate` in a terminal
2. Wait for "Ready. Press right ⌘ to dictate. Ctrl+C to stop."
3. Press and hold the hotkey in any application
4. Speak into your microphone
5. Release the hotkey — text appears in the active window
6. Press Ctrl+C in the terminal to stop the daemon

## Flags

| Flag                | Description                            | Default     |
| ------------------- | -------------------------------------- | ----------- |
| `--hotkey <key>`    | Hotkey to trigger recording            | `right-cmd` |
| `--language <code>` | Language code for transcription        | auto-detect |
| `--output-only`     | Print to stdout instead of auto-typing | false       |
| `--verbose`         | Show debug details                     | false       |
| `--help`            | Show usage information                 | -           |
| `--version`         | Show version                           | -           |

### Supported Hotkeys

`right-cmd`, `left-cmd`, `right-alt`, `left-alt`, `right-shift`, `left-shift`, `right-ctrl`, `left-ctrl`

## Permissions

### Accessibility

The daemon needs Accessibility permissions to detect global hotkeys and auto-type text.

1. Go to **System Settings > Privacy & Security > Accessibility**
2. Enable your terminal app (Terminal.app, iTerm2, etc.)

### Microphone

The daemon needs Microphone permissions to record audio.

1. Go to **System Settings > Privacy & Security > Microphone**
2. Enable your terminal app

## Architecture

```
[voice-dictate serve]
  → Check prerequisites (sox)
  → Download model if missing (~640MB, first run only)
  → Load model into memory (3-5s)
  → Register global hotkey
  → Wait for hotkey press
  → [Press] → sox records audio
  → [Release] → sherpa-onnx transcribes locally
  → osascript auto-types text into active window
  → Return to waiting
```

## Model

- **Model**: NVIDIA Parakeet TDT 0.6B v3 (INT8 quantized)
- **Size**: ~640MB
- **Languages**: Auto-detects among 25 European languages
- **Cached**: `~/.voice-dictate/models/`
- **Downloaded**: Automatically on first run from GitHub releases

## License

MIT
