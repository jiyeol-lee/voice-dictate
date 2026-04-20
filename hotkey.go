package main

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>
#include <stdio.h>
#include <pthread.h>

// Global state
static volatile int g_hotkeyPressed = 0;
static int g_targetKeyCode = -1;
static CFMachPortRef g_eventTap = NULL;
static CFRunLoopSourceRef g_runLoopSource = NULL;
static CFRunLoopRef g_runLoop = NULL;
static pthread_t g_runLoopThread;
static volatile int g_threadStarted = 0;
static pthread_mutex_t g_threadMutex = PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t g_threadCond = PTHREAD_COND_INITIALIZER;

// Helper: check if a specific modifier flag is set
static bool isModifierSet(uint64_t flags, int keyCode) {
    switch (keyCode) {
        case 0x36: // Right Command
        case 0x37: // Left Command
            return (flags & kCGEventFlagMaskCommand) != 0;
        case 0x3A: // Left Option
        case 0x3D: // Right Option
            return (flags & kCGEventFlagMaskAlternate) != 0;
        case 0x38: // Left Shift
        case 0x3C: // Right Shift
            return (flags & kCGEventFlagMaskShift) != 0;
        case 0x3B: // Left Control
        case 0x3E: // Right Control
            return (flags & kCGEventFlagMaskControl) != 0;
        default:
            return false;
    }
}

// Event tap callback
static CGEventRef eventTapCallback(CGEventTapProxy proxy, CGEventType type, CGEventRef event, void *refcon) {
    if (type == kCGEventFlagsChanged) {
        CGKeyCode keyCode = (CGKeyCode)CGEventGetIntegerValueField(event, kCGKeyboardEventKeycode);
        uint64_t flags = CGEventGetFlags(event);

        if ((int)keyCode == g_targetKeyCode) {
            bool isSet = isModifierSet(flags, g_targetKeyCode);
            g_hotkeyPressed = isSet ? 1 : 0;
        }
    }

    return event; // Pass through
}

// Run loop thread function
static void* runLoopThreadFunc(void* arg) {
    g_runLoop = CFRunLoopGetCurrent();

    if (g_runLoopSource != NULL) {
        CFRunLoopAddSource(g_runLoop, g_runLoopSource, kCFRunLoopDefaultMode);
    }

    CGEventTapEnable(g_eventTap, true);

    // Signal that the thread has started
    pthread_mutex_lock(&g_threadMutex);
    g_threadStarted = 1;
    pthread_cond_signal(&g_threadCond);
    pthread_mutex_unlock(&g_threadMutex);

    CFRunLoopRun();

    return NULL;
}

// Start the event tap
int startEventTap(int keyCode) {
    g_targetKeyCode = keyCode;
    g_hotkeyPressed = 0;
    g_runLoop = NULL;
    g_runLoopThread = 0;
    g_threadStarted = 0;

    CGEventMask eventMask = CGEventMaskBit(kCGEventFlagsChanged);

    g_eventTap = CGEventTapCreate(
        kCGHIDEventTap,          // Hardware-level (captures kCGEventFlagsChanged with keycode)
        kCGHeadInsertEventTap,
        kCGEventTapOptionDefault,
        eventMask,
        eventTapCallback,
        NULL
    );

    if (g_eventTap == NULL) {
        return -1;
    }

    g_runLoopSource = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, g_eventTap, 0);
    if (g_runLoopSource == NULL) {
        CFRelease(g_eventTap);
        g_eventTap = NULL;
        return -2;
    }

    // Start run loop in a separate thread
    if (pthread_create(&g_runLoopThread, NULL, runLoopThreadFunc, NULL) != 0) {
        CFRelease(g_runLoopSource);
        CFRelease(g_eventTap);
        g_eventTap = NULL;
        g_runLoopSource = NULL;
        return -3;
    }

    // Wait for thread to start
    pthread_mutex_lock(&g_threadMutex);
    while (!g_threadStarted) {
        pthread_cond_wait(&g_threadCond, &g_threadMutex);
    }
    pthread_mutex_unlock(&g_threadMutex);

    return 0;
}

// Stop the event tap
void stopEventTap() {
    if (g_eventTap != NULL) {
        if (g_runLoop != NULL) {
            CFRunLoopStop(g_runLoop);
        }

        // Wait for thread to exit
        pthread_join(g_runLoopThread, NULL);

        if (g_runLoopSource != NULL) {
            CFRelease(g_runLoopSource);
            g_runLoopSource = NULL;
        }

        CFRelease(g_eventTap);
        g_eventTap = NULL;
        g_runLoop = NULL;
    }
    g_targetKeyCode = -1;
    g_hotkeyPressed = 0;
}

// Check if hotkey is pressed
int isHotkeyPressed() {
    return g_hotkeyPressed;
}
*/
import "C"

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// HotkeyEvent represents a hotkey event.
type HotkeyEvent int

const (
	HotkeyPressed  HotkeyEvent = 1
	HotkeyReleased HotkeyEvent = 2
)

var (
	hotkeyChan   chan HotkeyEvent
	hotkeyMu     sync.Mutex
	hotkeyActive bool
	hotkeyCode   int
	hotkeyDone   chan struct{}
	hotkeyWg     sync.WaitGroup
)

// setupHotkey starts the CGEventTap and polling goroutine.
func setupHotkey(hotkey string, verbose bool) (chan HotkeyEvent, error) {
	hotkeyMu.Lock()
	defer hotkeyMu.Unlock()

	if hotkeyActive {
		return nil, fmt.Errorf("hotkey already registered")
	}

	keyCode, err := hotkeyToKeyCode(hotkey)
	if err != nil {
		return nil, err
	}

	hotkeyCode = int(keyCode)

	if verbose {
		fmt.Fprintf(os.Stderr, "Registering hotkey with keycode: 0x%02x\n", keyCode)
	}

	// Start the CGEventTap
	result := C.startEventTap(C.int(keyCode))
	if result != 0 {
		return nil, fmt.Errorf("failed to create event tap (error code: %d)\n"+
			"Make sure Accessibility permissions are granted for your terminal.\n"+
			"Go to: System Settings > Privacy & Security > Accessibility", result)
	}

	hotkeyChan = make(chan HotkeyEvent, 10)
	hotkeyActive = true
	hotkeyDone = make(chan struct{})
	hotkeyWg.Add(1)

	// Initialize the release signal channel
	initReleaseSignal()

	// Start polling goroutine
	go pollHotkeyEvents(verbose)

	return hotkeyChan, nil
}

// cleanupHotkey stops the hotkey polling safely.
func cleanupHotkey() {
	hotkeyMu.Lock()
	defer hotkeyMu.Unlock()

	if !hotkeyActive {
		return
	}

	hotkeyActive = false
	close(hotkeyDone)
	hotkeyWg.Wait()

	C.stopEventTap()

	if hotkeyChan != nil {
		close(hotkeyChan)
		hotkeyChan = nil
	}
}

// pollHotkeyEvents polls for hotkey state changes.
func pollHotkeyEvents(verbose bool) {
	defer hotkeyWg.Done()

	lastState := false

	for {
		select {
		case <-hotkeyDone:
			return
		default:
		}

		if !hotkeyActive {
			return
		}

		pressed := C.isHotkeyPressed() == 1

		if pressed && !lastState {
			if verbose {
				fmt.Fprintf(os.Stderr, "[hotkey] pressed\n")
			}
			select {
			case hotkeyChan <- HotkeyPressed:
			default:
			}
		} else if !pressed && lastState {
			if verbose {
				fmt.Fprintf(os.Stderr, "[hotkey] released\n")
			}
			signalRelease()

			select {
			case hotkeyChan <- HotkeyReleased:
			default:
			}
		}

		lastState = pressed
		time.Sleep(10 * time.Millisecond)
	}
}

// hotkeyToKeyCode maps a hotkey name to a macOS virtual keycode.
func hotkeyToKeyCode(hotkey string) (uint32, error) {
	switch hotkey {
	case "right-cmd", "right-command":
		return 0x36, nil
	case "left-cmd", "left-command":
		return 0x37, nil
	case "right-alt", "right-option":
		return 0x3D, nil
	case "left-alt", "left-option":
		return 0x3A, nil
	case "right-shift":
		return 0x3C, nil
	case "left-shift":
		return 0x38, nil
	case "right-ctrl", "right-control":
		return 0x3E, nil
	case "left-ctrl", "left-control":
		return 0x3B, nil
	default:
		return 0, fmt.Errorf(
			"unsupported hotkey: %s (supported: right-cmd, left-cmd, right-alt, left-alt, right-shift, left-shift, right-ctrl, left-ctrl)",
			hotkey,
		)
	}
}
