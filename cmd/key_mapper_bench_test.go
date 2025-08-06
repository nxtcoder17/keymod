package main

import (
	"io"
	"testing"

	evdev "github.com/holoplot/go-evdev"
	"github.com/nxtcoder17/fastlog"
)

func init() {
	// Disable logging for benchmarks by writing to io.Discard
	logger = fastlog.New(fastlog.Options{
		Writer:        io.Discard,
		ShowCaller:    false,
		ShowDebugLogs: false,
		ShowTimestamp: false,
		EnableColors:  false,
		Format:        fastlog.ConsoleFormat,
	})
}

func BenchmarkKeyMapper_TapBehavior(b *testing.B) {
	// Setup
	mockDevice := NewMockInputDevice("test-keyboard", "/dev/input/event0")
	cfg := &ParsedConfig{
		ModMap: map[string]*TapAndHold{
			"KEY_CAPSLOCK": {
				Key:  "KEY_CAPSLOCK",
				Tap:  "ESC",
				Hold: "LEFTCTRL",
			},
			"KEY_SPACE": {
				Key:  "KEY_SPACE",
				Tap:  "SPACE",
				Hold: "LEFTSHIFT",
			},
		},
	}
	
	kbd := &MyModKeyboard{
		device:    mockDevice,
		keyDownCh: make(chan *evdev.InputEvent, 1),
		cfg:       cfg,
	}

	// Benchmark tap events
	tapEvents := []*evdev.InputEvent{
		{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyDown},
		{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyUp},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, event := range tapEvents {
			kbd.onEvent(event)
		}
		mockDevice.ClearWrittenEvents()
		kbd.counter = 0
		kbd.downCounter = 0
		kbd.prev = nil
	}
}

func BenchmarkKeyMapper_HoldBehavior(b *testing.B) {
	// Setup
	mockDevice := NewMockInputDevice("test-keyboard", "/dev/input/event0")
	cfg := &ParsedConfig{
		ModMap: map[string]*TapAndHold{
			"KEY_CAPSLOCK": {
				Key:  "KEY_CAPSLOCK",
				Tap:  "ESC",
				Hold: "LEFTCTRL",
			},
		},
	}
	
	kbd := &MyModKeyboard{
		device:    mockDevice,
		keyDownCh: make(chan *evdev.InputEvent, 1),
		cfg:       cfg,
	}

	// Benchmark hold events (CAPS+A)
	holdEvents := []*evdev.InputEvent{
		{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyDown},
		{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyDown},
		{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyUp},
		{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyUp},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, event := range holdEvents {
			kbd.onEvent(event)
		}
		mockDevice.ClearWrittenEvents()
		kbd.counter = 0
		kbd.downCounter = 0
		kbd.prev = nil
	}
}

func BenchmarkKeyMapper_PassthroughKeys(b *testing.B) {
	// Setup
	mockDevice := NewMockInputDevice("test-keyboard", "/dev/input/event0")
	cfg := &ParsedConfig{
		ModMap: map[string]*TapAndHold{
			"KEY_CAPSLOCK": {
				Key:  "KEY_CAPSLOCK",
				Tap:  "ESC",
				Hold: "LEFTCTRL",
			},
		},
	}
	
	kbd := &MyModKeyboard{
		device:    mockDevice,
		keyDownCh: make(chan *evdev.InputEvent, 1),
		cfg:       cfg,
	}

	// Benchmark unmapped key passthrough
	passthroughEvents := []*evdev.InputEvent{
		{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyDown},
		{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyUp},
		{Type: evdev.EV_KEY, Code: evdev.KEY_B, Value: KeyDown},
		{Type: evdev.EV_KEY, Code: evdev.KEY_B, Value: KeyUp},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, event := range passthroughEvents {
			kbd.onEvent(event)
		}
		mockDevice.ClearWrittenEvents()
		kbd.counter = 0
		kbd.downCounter = 0
		kbd.prev = nil
	}
}

func BenchmarkKeyMapper_ComplexSequence(b *testing.B) {
	// Setup
	mockDevice := NewMockInputDevice("test-keyboard", "/dev/input/event0")
	cfg := &ParsedConfig{
		ModMap: map[string]*TapAndHold{
			"KEY_CAPSLOCK": {
				Key:  "KEY_CAPSLOCK",
				Tap:  "ESC",
				Hold: "LEFTCTRL",
			},
			"KEY_SPACE": {
				Key:  "KEY_SPACE",
				Tap:  "SPACE",
				Hold: "LEFTSHIFT",
			},
		},
	}
	
	kbd := &MyModKeyboard{
		device:    mockDevice,
		keyDownCh: make(chan *evdev.InputEvent, 1),
		cfg:       cfg,
	}

	// Complex sequence: tap CAPS, hold SPACE+A, tap CAPS
	complexEvents := []*evdev.InputEvent{
		// Tap CAPSLOCK
		{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyDown},
		{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyUp},
		// Hold SPACE with A
		{Type: evdev.EV_KEY, Code: evdev.KEY_SPACE, Value: KeyDown},
		{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyDown},
		{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyUp},
		{Type: evdev.EV_KEY, Code: evdev.KEY_SPACE, Value: KeyUp},
		// Tap CAPSLOCK again
		{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyDown},
		{Type: evdev.EV_KEY, Code: evdev.KEY_CAPSLOCK, Value: KeyUp},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, event := range complexEvents {
			kbd.onEvent(event)
		}
		mockDevice.ClearWrittenEvents()
		kbd.counter = 0
		kbd.downCounter = 0
		kbd.prev = nil
	}
}

func BenchmarkParseKeyCode(b *testing.B) {
	testKeys := []string{
		"KEY_A", "A", "KEY_SPACE", "SPACE", 
		"KEY_LEFTCTRL", "LEFTCTRL", "KEY_ESC", "ESC",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, key := range testKeys {
			_ = parseKeyCode(key)
		}
	}
}

func BenchmarkDispatchKeyCodes_Single(b *testing.B) {
	mockDevice := NewMockInputDevice("test-keyboard", "/dev/input/event0")
	kbd := &MyModKeyboard{
		device:    mockDevice,
		keyDownCh: make(chan *evdev.InputEvent, 1),
		cfg:       &ParsedConfig{ModMap: make(map[string]*TapAndHold)},
	}

	event := &evdev.InputEvent{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyDown}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kbd.dispatchKeyCodes(event)
		mockDevice.ClearWrittenEvents()
	}
}

func BenchmarkDispatchKeyCodes_Multiple(b *testing.B) {
	mockDevice := NewMockInputDevice("test-keyboard", "/dev/input/event0")
	kbd := &MyModKeyboard{
		device:    mockDevice,
		keyDownCh: make(chan *evdev.InputEvent, 1),
		cfg:       &ParsedConfig{ModMap: make(map[string]*TapAndHold)},
	}

	events := []*evdev.InputEvent{
		{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyDown},
		{Type: evdev.EV_KEY, Code: evdev.KEY_A, Value: KeyUp},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kbd.dispatchKeyCodes(events...)
		mockDevice.ClearWrittenEvents()
	}
}

// Benchmark for configuration lookup performance
func BenchmarkConfigLookup(b *testing.B) {
	cfg := &ParsedConfig{
		ModMap: map[string]*TapAndHold{
			"KEY_CAPSLOCK": {Key: "KEY_CAPSLOCK", Tap: "ESC", Hold: "LEFTCTRL"},
			"KEY_SPACE":    {Key: "KEY_SPACE", Tap: "SPACE", Hold: "LEFTSHIFT"},
			"KEY_TAB":      {Key: "KEY_TAB", Tap: "TAB", Hold: "LEFTALT"},
			"KEY_ENTER":    {Key: "KEY_ENTER", Tap: "ENTER", Hold: "RIGHTCTRL"},
			"KEY_LSHIFT":   {Key: "KEY_LSHIFT", Tap: "LSHIFT", Hold: "LEFTMETA"},
		},
	}

	// Mix of mapped and unmapped keys
	testKeys := []string{
		"KEY_CAPSLOCK", "KEY_A", "KEY_SPACE", "KEY_B",
		"KEY_TAB", "KEY_C", "KEY_ENTER", "KEY_D",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, key := range testKeys {
			_, _ = cfg.ModMap[key]
		}
	}
}