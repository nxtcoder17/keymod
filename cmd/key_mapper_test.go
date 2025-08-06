package main

import (
	"os"
	"testing"
	"time"

	evdev "github.com/holoplot/go-evdev"
	"github.com/nxtcoder17/fastlog"
)

func init() {
	// Initialize logger for tests
	logger = fastlog.New(fastlog.Options{
		Writer:        os.Stderr,
		ShowCaller:    false,
		ShowDebugLogs: false,
		ShowTimestamp: false,
		EnableColors:  false,
		Format:        fastlog.ConsoleFormat,
	})
}

func TestMyModKeyboard_TapBehavior(t *testing.T) {
	tests := []struct {
		name           string
		config         *ParsedConfig
		inputEvents    []*evdev.InputEvent
		expectedEvents []*evdev.InputEvent
		description    string
	}{
		{
			name: "capslock_tap_sends_esc",
			config: &ParsedConfig{
				ModMap: map[string]*TapAndHold{
					"KEY_CAPSLOCK": {
						Key:  "KEY_CAPSLOCK",
						Tap:  "ESC",
						Hold: "LEFTCTRL",
					},
				},
			},
			inputEvents: []*evdev.InputEvent{
				{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyDown},
				{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyUp},
			},
			expectedEvents: []*evdev.InputEvent{
				{Type: evdev.EV_KEY, Code: evdev.KEY_ESC, Value: KeyDown},
				{Type: evdev.EV_KEY, Code: evdev.KEY_ESC, Value: KeyUp},
				{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT, Value: 0},
			},
			description: "CAPSLOCK tap should send ESC key press",
		},
		{
			name: "space_tap_sends_space",
			config: &ParsedConfig{
				ModMap: map[string]*TapAndHold{
					"KEY_SPACE": {
						Key:  "KEY_SPACE",
						Tap:  "SPACE",
						Hold: "LEFTSHIFT",
					},
				},
			},
			inputEvents: []*evdev.InputEvent{
				{Type: evdev.EV_KEY, Code: evdev.KEY_SPACE, Value: KeyDown},
				{Type: evdev.EV_KEY, Code: evdev.KEY_SPACE, Value: KeyUp},
			},
			expectedEvents: []*evdev.InputEvent{
				{Type: evdev.EV_KEY, Code: evdev.KEY_SPACE, Value: KeyDown},
				{Type: evdev.EV_KEY, Code: evdev.KEY_SPACE, Value: KeyUp},
				{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT, Value: 0},
			},
			description: "SPACE tap should send SPACE key press",
		},
		{
			name: "unmapped_key_passthrough",
			config: &ParsedConfig{
				ModMap: map[string]*TapAndHold{
					"KEY_CAPSLOCK": {
						Key:  "KEY_CAPSLOCK",
						Tap:  "ESC",
						Hold: "LEFTCTRL",
					},
				},
			},
			inputEvents: []*evdev.InputEvent{
				{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyDown},
				{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyUp},
			},
			expectedEvents: []*evdev.InputEvent{
				{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyDown},
				{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT, Value: 0},
				{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyUp},
				{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT, Value: 0},
			},
			description: "Unmapped keys should pass through unchanged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock device
			mockDevice := NewMockInputDevice("test-keyboard", "/dev/input/event0")

			// Create keyboard handler
			kbd := &MyModKeyboard{
				device:    mockDevice,
				keyDownCh: make(chan *evdev.InputEvent, 1),
				cfg:       tt.config,
			}

			// Process input events
			for _, event := range tt.inputEvents {
				kbd.onEvent(event)
			}

			// Allow a small delay for async operations
			time.Sleep(10 * time.Millisecond)

			// Get written events
			writtenEvents := mockDevice.GetWrittenEvents()

			// Verify the number of events
			if len(writtenEvents) != len(tt.expectedEvents) {
				t.Errorf("%s: expected %d events, got %d", tt.description, len(tt.expectedEvents), len(writtenEvents))
				t.Logf("Expected events: %+v", tt.expectedEvents)
				t.Logf("Written events: %+v", writtenEvents)
				return
			}

			// Verify each event
			for i, expected := range tt.expectedEvents {
				actual := writtenEvents[i]
				if actual.Type != expected.Type || actual.Code != expected.Code || actual.Value != expected.Value {
					t.Errorf("%s: event %d mismatch\nexpected: %+v\ngot: %+v",
						tt.description, i, expected, actual)
				}
			}
		})
	}
}

func TestMyModKeyboard_HoldBehavior(t *testing.T) {
	tests := []struct {
		name           string
		config         *ParsedConfig
		inputEvents    []*evdev.InputEvent
		expectedEvents []*evdev.InputEvent
		description    string
	}{
		{
			name: "capslock_hold_with_key_sends_ctrl",
			config: &ParsedConfig{
				ModMap: map[string]*TapAndHold{
					"KEY_CAPSLOCK": {
						Key:  "KEY_CAPSLOCK",
						Tap:  "ESC",
						Hold: "LEFTCTRL",
					},
				},
			},
			inputEvents: []*evdev.InputEvent{
				{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyDown},
				{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyDown},
				{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyUp},
				{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyUp},
			},
			expectedEvents: []*evdev.InputEvent{
				// CAPSLOCK down - no immediate output
				// A down - triggers CTRL modifier
				{Type: evdev.EV_KEY, Code: evdev.KEY_LEFTCTRL, Value: KeyDown},
				{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT, Value: 0},
				{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyDown},
				{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT, Value: 0},
				// A up
				{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyUp},
				{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT, Value: 0},
				// CAPSLOCK up - release CTRL
				{Type: evdev.EV_KEY, Code: evdev.KEY_LEFTCTRL, Value: KeyUp},
				{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT, Value: 0},
			},
			description: "CAPSLOCK hold + A should send CTRL+A",
		},
		{
			name: "space_hold_with_key_sends_shift",
			config: &ParsedConfig{
				ModMap: map[string]*TapAndHold{
					"KEY_SPACE": {
						Key:  "KEY_SPACE",
						Tap:  "SPACE",
						Hold: "LEFTSHIFT",
					},
				},
			},
			inputEvents: []*evdev.InputEvent{
				{Type: evdev.EV_KEY, Code: evdev.KEY_SPACE, Value: KeyDown},
				{Type: evdev.EV_KEY, Code: evdev.KEY_B, Value: KeyDown},
				{Type: evdev.EV_KEY, Code: evdev.KEY_B, Value: KeyUp},
				{Type: evdev.EV_KEY, Code: evdev.KEY_SPACE, Value: KeyUp},
			},
			expectedEvents: []*evdev.InputEvent{
				// SPACE down - no immediate output
				// B down - triggers SHIFT modifier
				{Type: evdev.EV_KEY, Code: evdev.KEY_LEFTSHIFT, Value: KeyDown},
				{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT, Value: 0},
				{Type: evdev.EV_KEY, Code: evdev.KEY_B, Value: KeyDown},
				{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT, Value: 0},
				// B up
				{Type: evdev.EV_KEY, Code: evdev.KEY_B, Value: KeyUp},
				{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT, Value: 0},
				// SPACE up - release SHIFT
				{Type: evdev.EV_KEY, Code: evdev.KEY_LEFTSHIFT, Value: KeyUp},
				{Type: evdev.EV_SYN, Code: evdev.SYN_REPORT, Value: 0},
			},
			description: "SPACE hold + B should send SHIFT+B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock device
			mockDevice := NewMockInputDevice("test-keyboard", "/dev/input/event0")

			// Create keyboard handler
			kbd := &MyModKeyboard{
				device:    mockDevice,
				keyDownCh: make(chan *evdev.InputEvent, 1),
				cfg:       tt.config,
			}

			// Process input events
			for _, event := range tt.inputEvents {
				kbd.onEvent(event)
			}

			// Allow a small delay for async operations
			time.Sleep(10 * time.Millisecond)

			// Get written events
			writtenEvents := mockDevice.GetWrittenEvents()

			// Verify the number of events
			if len(writtenEvents) != len(tt.expectedEvents) {
				t.Errorf("%s: expected %d events, got %d", tt.description, len(tt.expectedEvents), len(writtenEvents))
				t.Logf("Expected events: %+v", tt.expectedEvents)
				t.Logf("Written events: %+v", writtenEvents)
				return
			}

			// Verify each event
			for i, expected := range tt.expectedEvents {
				actual := writtenEvents[i]
				if actual.Type != expected.Type || actual.Code != expected.Code || actual.Value != expected.Value {
					t.Errorf("%s: event %d mismatch\nexpected: %+v\ngot: %+v",
						tt.description, i, expected, actual)
				}
			}
		})
	}
}

func TestMyModKeyboard_ComplexSequences(t *testing.T) {
	tests := []struct {
		name          string
		config        *ParsedConfig
		inputEvents   []*evdev.InputEvent
		expectedCodes []evdev.EvCode
		description   string
	}{
		{
			name: "multiple_taps_in_sequence",
			config: &ParsedConfig{
				ModMap: map[string]*TapAndHold{
					"KEY_CAPSLOCK": {
						Key:  "KEY_CAPSLOCK",
						Tap:  "ESC",
						Hold: "LEFTCTRL",
					},
				},
			},
			inputEvents: []*evdev.InputEvent{
				// First tap
				{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyDown},
				{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyUp},
				// Second tap
				{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyDown},
				{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyUp},
			},
			expectedCodes: []evdev.EvCode{
				evdev.KEY_ESC, // down
				evdev.KEY_ESC, // up
				evdev.KEY_ESC, // down
				evdev.KEY_ESC, // up
			},
			description: "Multiple CAPSLOCK taps should send multiple ESC presses",
		},
		{
			name: "tap_then_hold_sequence",
			config: &ParsedConfig{
				ModMap: map[string]*TapAndHold{
					"KEY_CAPSLOCK": {
						Key:  "KEY_CAPSLOCK",
						Tap:  "ESC",
						Hold: "LEFTCTRL",
					},
				},
			},
			inputEvents: []*evdev.InputEvent{
				// First: tap
				{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyDown},
				{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyUp},
				// Second: hold
				{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyDown},
				{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyDown},
				{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyUp},
				{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyUp},
			},
			expectedCodes: []evdev.EvCode{
				evdev.KEY_ESC,      // tap: down
				evdev.KEY_ESC,      // tap: up
				evdev.KEY_LEFTCTRL, // hold: ctrl down
				evdev.KEY_A,        // a down
				evdev.KEY_A,        // a up
				evdev.KEY_LEFTCTRL, // hold: ctrl up
			},
			description: "Tap followed by hold should work correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock device
			mockDevice := NewMockInputDevice("test-keyboard", "/dev/input/event0")

			// Create keyboard handler
			kbd := &MyModKeyboard{
				device:    mockDevice,
				keyDownCh: make(chan *evdev.InputEvent, 1),
				cfg:       tt.config,
			}

			// Process input events
			for _, event := range tt.inputEvents {
				kbd.onEvent(event)
			}

			// Allow a small delay for async operations
			time.Sleep(10 * time.Millisecond)

			// Get written events
			writtenEvents := mockDevice.GetWrittenEvents()

			// Extract key codes (ignore SYN events)
			var actualCodes []evdev.EvCode
			for _, event := range writtenEvents {
				if event.Type == evdev.EV_KEY {
					actualCodes = append(actualCodes, event.Code)
				}
			}

			// Verify the key codes match
			if len(actualCodes) != len(tt.expectedCodes) {
				t.Errorf("%s: expected %d key events, got %d", tt.description, len(tt.expectedCodes), len(actualCodes))
				t.Logf("Expected codes: %+v", tt.expectedCodes)
				t.Logf("Actual codes: %+v", actualCodes)
				return
			}

			for i, expected := range tt.expectedCodes {
				if actualCodes[i] != expected {
					t.Errorf("%s: key code %d mismatch\nexpected: %v\ngot: %v",
						tt.description, i, expected, actualCodes[i])
				}
			}
		})
	}
}
