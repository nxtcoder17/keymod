package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unicode"

	evdev "github.com/holoplot/go-evdev"
	"github.com/nxtcoder17/fastlog"
)

const (
	KeyUp   int32 = 0
	KeyDown int32 = 1
	KeyHold int32 = 2
)

var logger fastlog.Logger

func parseEventType(t int32) string {
	switch t {
	case KeyUp:
		return "KEY_UP"
	case KeyDown:
		return "KEY_DOWN"
	case KeyHold:
		return "KEY_HOLD"
	}
	return ""
}

func isKeyDown(event *evdev.InputEvent) bool {
	return event.Value == KeyDown
}

func parseKeyCode(s string) evdev.EvCode {
	if strings.HasPrefix(s, "KEY_") {
		return evdev.KEYFromString[s]
	}
	return evdev.KEYFromString["KEY_"+s]
}

func eventToString(ev *evdev.InputEvent) string {
	return fmt.Sprintf("[type] %-12s[key] %s", parseEventType(ev.Value), ev.CodeName())
}

func findAllKeyboards() ([]*evdev.InputDevice, error) {
	devicePaths, err := evdev.ListDevicePaths()
	if err != nil {
		return nil, fmt.Errorf("failed to list input devices: %w", err)
	}

	var keyboards []*evdev.InputDevice

	for _, devpath := range devicePaths {
		dev, err := evdev.Open(devpath.Path)
		if err != nil {
			logger.Debug("failed to open device", "path", devpath.Path, "error", err)
			continue
		}

		// Check if device has KEY capabilities and can handle keyboard events
		supportedKeys := dev.CapableEvents(evdev.EV_KEY)

		if len(supportedKeys) == 0 {
			dev.Close()
			continue
		}

		isKeyboard := false

		keyMap := make(map[evdev.EvCode]bool)
		for _, key := range supportedKeys {
			keyMap[key] = true
		}

		// NOTE: verifies if the device has some test keys
		testKeys := []evdev.EvCode{evdev.KEY_A, evdev.KEY_SPACE, evdev.KEY_ENTER}
		for _, key := range testKeys {
			if keyMap[key] {
				isKeyboard = true
				break
			}
		}

		if !isKeyboard {
			dev.Close()
			continue
		}

		keyboards = append(keyboards, dev)
		name, _ := dev.Name()
		logger.Info("Found keyboard device", "name", name, "path", devpath.Path)
	}

	if len(keyboards) == 0 {
		return nil, fmt.Errorf("no keyboard devices found")
	}

	slices.SortFunc(keyboards, func(a, b *evdev.InputDevice) int {
		return extractDeviceNumber(a) - extractDeviceNumber(b)
	})

	return keyboards, nil
}

func extractDeviceNumber(s *evdev.InputDevice) int {
	devPath := s.Path()

	start := len(devPath) - 1
	for i := start; i >= 0; i-- {
		if !unicode.IsDigit(rune(devPath[i])) {
			break
		}
		start = i
	}

	n, err := strconv.Atoi(devPath[start:])
	if err != nil {
		return 0
	}
	return n
}

func cloneDevice(devicePath string, cfg *ParsedConfig) (*evdev.InputDevice, error) {
	targetDev, err := evdev.Open(devicePath)
	if err != nil {
		fmt.Printf("failed to open target device for cloning: %s", err.Error())
		return nil, err
	}
	defer targetDev.Close()

	keyCodes := targetDev.CapableEvents(evdev.EV_KEY)
	seenKeys := make(map[evdev.EvCode]bool, len(keyCodes)+len(cfg.ModMap)*3)
	for _, code := range keyCodes {
		seenKeys[code] = true
	}

	for _, mapping := range cfg.ModMap {
		seenKeys[parseKeyCode(mapping.Key)] = true
		seenKeys[parseKeyCode(mapping.Tap)] = true
		seenKeys[parseKeyCode(mapping.Hold)] = true
	}

	keyCodes = keyCodes[:0]
	for code := range seenKeys {
		keyCodes = append(keyCodes, code)
	}

	clonedDev, err := evdev.CreateDevice(
		fmt.Sprintf("keymod virtual keyboard - %s", must(targetDev.Name())),
		evdev.InputID{
			BusType: evdev.BUS_USB,
			Vendor:  0xfeed,
			Product: 0x0001,
			Version: 1,
		},
		map[evdev.EvType][]evdev.EvCode{
			evdev.EV_KEY: keyCodes,
		},
	)
	if err != nil {
		fmt.Printf("failed to clone device: %s", err.Error())
		return nil, err
	}
	return clonedDev, nil
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func eventKeyPress(code evdev.EvCode) []*evdev.InputEvent {
	return []*evdev.InputEvent{
		{Type: evdev.EV_KEY, Code: code, Value: KeyDown},
		{Type: evdev.EV_KEY, Code: code, Value: KeyUp},
	}
}

func eventKeyUp(key evdev.EvCode) *evdev.InputEvent {
	return &evdev.InputEvent{
		Type: evdev.EV_KEY, Code: key, Value: KeyUp,
	}
}

func eventKeyDown(key evdev.EvCode) *evdev.InputEvent {
	return &evdev.InputEvent{
		Type: evdev.EV_KEY, Code: key, Value: KeyDown,
	}
}

func withModifierKey(modifier evdev.EvCode, events ...*evdev.InputEvent) []*evdev.InputEvent {
	mods := make([]*evdev.InputEvent, 0, 2+len(events))
	mods = append(mods, eventKeyDown(modifier))
	mods = append(mods, events...)
	return mods
}

func (kdb *MyModKeyboard) dispatchKeyCodes(events ...*evdev.InputEvent) {
	if len(events) == 0 {
		return
	}
	for i := range events {
		kdb.device.WriteOne(events[i])
	}

	kdb.device.WriteOne(&evdev.InputEvent{
		Type:  evdev.EV_SYN,
		Code:  evdev.SYN_REPORT,
		Value: 0,
	})
}

func (kdb *MyModKeyboard) dispatchRawKeyCodes(events ...*evdev.InputEvent) {
	if len(events) == 0 {
		return
	}
	for i := range events {
		kdb.device.WriteOne(events[i])
	}
}

type pendingPress struct {
	mapping  *TapAndHold
	holdSent bool
}

type MyModKeyboard struct {
	keyDownCh chan *evdev.InputEvent
	cfg       *ParsedConfig

	device InputDevice

	onModHold    func()
	onModRelease func()

	sendHoldEvent context.CancelFunc
	counter       int
	downCounter   int
	prev          *TapAndHold

	// Per-physical-key state. This is what makes combos (e.g. ALT+T)
	// and multiple simultaneous remaps (Mac-like ALT->CTRL plus
	// SUPER->ALT) work instead of the old single `prev` slot.
	physicalDown map[evdev.EvCode]bool
	pending      map[evdev.EvCode]*pendingPress
	holdActive   map[evdev.EvCode]bool
}

func (kbd *MyModKeyboard) ensureState() {
	if kbd.physicalDown == nil {
		kbd.physicalDown = make(map[evdev.EvCode]bool)
	}
	if kbd.pending == nil {
		kbd.pending = make(map[evdev.EvCode]*pendingPress)
	}
	if kbd.holdActive == nil {
		kbd.holdActive = make(map[evdev.EvCode]bool)
	}
}

func (m *MyModKeyboard) ShutDown() error {
	return m.device.Close()
}

func (kbd *MyModKeyboard) onEvent(event *evdev.InputEvent) {
	if !strings.HasPrefix(event.CodeName(), "KEY_") {
		return
	}

	kbd.ensureState()

	kbd.counter += 1
	if isKeyDown(event) {
		kbd.downCounter += 1
	}

	logger := logger.With("event", eventToString(event), "kbd.counter", kbd.counter, "kbd.downCounter", kbd.downCounter)

	// Kernel autorepeat for a mapped key carries no new information:
	// the key is already physically down. Swallow it so a held
	// modifier doesn't re-promote or a tap doesn't double-fire.
	// Repeat of a *chord* key (e.g. holding T in ALT+T) is a non-mapped
	// key and still falls through to passthrough below, so key repeat
	// keeps working where it should.
	if _, ok := kbd.cfg.ModMap[event.CodeName()]; ok && event.Type == evdev.EV_KEY && event.Value == KeyHold {
		return
	}

	// Any *other* key going down promotes all still-pending mapped keys
	// to HOLD. This single rule is what makes combos work: ALT DOWN,
	// T DOWN => CTRL DOWN, T DOWN. It also handles several pending
	// remaps at once (ALT+SUPER+T).
	if isKeyDown(event) {
		for code, p := range kbd.pending {
			if code == event.Code {
				continue
			}
			if !p.holdSent {
				logger.Info("dispatching [HOLD]", "keycode", evdev.KEYToString[parseKeyCode(p.mapping.Hold)])
				kbd.dispatchKeyCodes(eventKeyDown(parseKeyCode(p.mapping.Hold)))
				p.holdSent = true
				kbd.holdActive[code] = true
			}
		}
		// A pending entry that already sent HOLD is no longer pending.
		for code, p := range kbd.pending {
			if p.holdSent {
				delete(kbd.pending, code)
			}
		}
	}

	if tapAndHold, ok := kbd.cfg.ModMap[event.CodeName()]; ok && event.Type == evdev.EV_KEY {
		switch event.Value {
		case KeyDown:
			// Bounce / duplicate DOWN while physically down: ignore.
			// Previously this false-promoted to HOLD and later produced
			// an extra TAP (double-space).
			if kbd.physicalDown[event.Code] {
				return
			}
			kbd.physicalDown[event.Code] = true
			logger.Info("modkey [DOWN]")
			// Copy the mapping: never mutate the shared ModMap entry,
			// otherwise overlapping presses corrupt each other.
			m := *tapAndHold
			kbd.pending[event.Code] = &pendingPress{mapping: &m}
			kbd.prev = tapAndHold
			tapAndHold.pressedCounter = kbd.downCounter
		case KeyUp:
			// Duplicate / stale UP with no matching DOWN: ignore.
			// This was the double-space panic path: the second UP
			// re-entered TAP with kbd.prev == nil (key_mapper.go:282).
			if !kbd.physicalDown[event.Code] {
				return
			}
			kbd.physicalDown[event.Code] = false
			logger.Info("modkey [UP]", "kbd.downCounter", kbd.downCounter)
			if p, stillPending := kbd.pending[event.Code]; stillPending {
				// No other key went down in between => TAP.
				delete(kbd.pending, event.Code)
				kbd.prev = nil
				logger.Info("dispatching [TAP]", "keycode", evdev.KEYToString[parseKeyCode(p.mapping.Tap)])
				kbd.dispatchKeyCodes(eventKeyPress(parseKeyCode(p.mapping.Tap))...)
				return
			}
			if kbd.holdActive[event.Code] {
				delete(kbd.holdActive, event.Code)
				kbd.prev = nil
				kbd.dispatchKeyCodes(eventKeyUp(parseKeyCode(tapAndHold.Hold)))
				return
			}
			// UP with no pending TAP and no active HOLD (e.g. state was
			// cleared by a previous release): nothing to emit.
			kbd.prev = nil
		}

		return
	}

	logger.Info("non-modkey")
	kbd.dispatchKeyCodes(event)
}

func Start(ctx context.Context, keyboard *evdev.InputDevice) error {
	logger.Info("[STARTED] listening on", "keyboard", must(keyboard.Name()))
	defer logger.Info("[STOPPED] listening on", "keyboard", must(keyboard.Name()))

	c, err := LoadConfig(configFile)
	if err != nil {
		panic(err)
	}

	clone, err := cloneDevice(keyboard.Path(), c)
	if err != nil {
		logger.Error("failed to clone device", "err", err)
		return err
	}
	defer clone.Close()

	if err := keyboard.Grab(); err != nil {
		logger.Error("failed to grab original keyboard", "err", err)
		return err
	}
	defer keyboard.Ungrab()

	eventsCh := make(chan *evdev.InputEvent, 1)

	mykb := &MyModKeyboard{
		device:    &inputDeviceWrapper{clone},
		keyDownCh: make(chan *evdev.InputEvent, 1),
		cfg:       c,
	}

	go func() {
		for ev := range eventsCh {
			if strings.HasPrefix(ev.CodeName(), "KEY_") {
				if debug {
					logger.Debug("keyboard input", "event", eventToString(ev))
				}
			}
		}
	}()

	go func() {
		<-mykb.keyDownCh
	}()

	for ctx.Err() == nil {
		ev, err := keyboard.ReadOne()
		if err != nil {
			logger.Error("while reading event from source keyboard", "err", err)
			return err
		}

		eventsCh <- ev
		mykb.onEvent(ev)
	}

	return nil
}

var (
	debug bool
	first bool

	configFile string
)

func main() {
	flag.BoolVar(&debug, "debug", false, "--debug")
	flag.BoolVar(&first, "first", false, "use only the first keyboard that generates an event")

	xdgDataDir := os.Getenv("XDG_CONFIG_HOME")
	if xdgDataDir == "" {
		xdgDataDir = filepath.Join(os.Getenv("HOME"), ".config")
	}

	flag.StringVar(&configFile, "config", filepath.Join(xdgDataDir, "keymod", "config.toml"), "--config <path-to-keymod-config>")

	flag.Parse()

	if value, ok := os.LookupEnv("KEYMOD_CONFIG_FILE"); ok {
		configFile = value
	}

	logger = fastlog.New().DebugMode(debug).Console()

	logger.Info("CONFIG", "file", configFile)

	keyboards, err := findAllKeyboards()
	if err != nil {
		logger.Error("failed to find keyboard", "err", err)
		os.Exit(1)
	}

	ctx, cf := signal.NotifyContext(context.TODO(), syscall.SIGINT, syscall.SIGTERM)
	defer cf()

	if first && len(keyboards) > 1 {
		logger.Info("Waiting for first keyboard event to select device...")

		firstEventCh := make(chan *evdev.InputDevice, len(keyboards))

		firstEventCtx, firstEventCancel := context.WithCancel(ctx)

		for i := range keyboards {
			keyboard := keyboards[i]

			if err := keyboard.Grab(); err != nil {
				logger.Error("failed to grab original keyboard", "err", err)
				panic(err)
			}

			go func(kb *evdev.InputDevice) {
				defer kb.Ungrab()
				ev, err := kb.ReadOne()
				if err != nil {
					return
				}

				if firstEventCtx.Err() == nil {
					logger.Info("First event detected", "keyboard", must(kb.Name()), "event", eventToString(ev))
					firstEventCh <- kb
				}
			}(keyboard)
		}

		var selectedKeyboard *evdev.InputDevice
		select {
		case kb := <-firstEventCh:
			selectedKeyboard = kb
			firstEventCancel()
			close(firstEventCh)
		case <-ctx.Done():
			logger.Info("Interrupted before keyboard selection")
			os.Exit(0)
		}

		if err := Start(ctx, selectedKeyboard); err != nil {
			logger.Error("FAILED, got", "err", err, "keyboard", must(selectedKeyboard.Name()))
		}

		return
	}

	var wg sync.WaitGroup
	for i := range keyboards {
		keyboard := keyboards[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := Start(ctx, keyboard); err != nil {
				logger.Error("FAILED, got", "err", err, "keyboard", must(keyboard.Name()))
			}
		}()
	}

	wg.Wait()
}
