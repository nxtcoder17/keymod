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

func cloneDevice(devicePath string) (*evdev.InputDevice, error) {
	targetDev, err := evdev.Open(devicePath)
	if err != nil {
		fmt.Printf("failed to open target device for cloning: %s", err.Error())
		return nil, err
	}
	defer targetDev.Close()

	clonedDev, err := evdev.CloneDevice(fmt.Sprintf("CLONE - %s", must(targetDev.Name())), targetDev)
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
}

func (m *MyModKeyboard) ShutDown() error {
	return m.device.Close()
}

func (kbd *MyModKeyboard) onEvent(event *evdev.InputEvent) {
	if !strings.HasPrefix(event.CodeName(), "KEY_") {
		kbd.dispatchKeyCodes(event)
		return
	}

	kbd.counter += 1
	if isKeyDown(event) {
		kbd.downCounter += 1
	}

	logger := logger.With("event", eventToString(event), "kbd.prev", kbd.prev == nil, "kbd.counter", kbd.counter, "kbd.downCounter", kbd.downCounter)

	if kbd.prev != nil && kbd.prev.pressedCounter+1 == kbd.downCounter && isKeyDown(event) {
		logger.Info("dispatching [HOLD]", "keycode", evdev.KEYToString[parseKeyCode(kbd.prev.Hold)])
		kbd.dispatchKeyCodes(eventKeyDown(parseKeyCode(kbd.prev.Hold)))
		kbd.prev = nil
	}

	if tapAndHold, ok := kbd.cfg.ModMap[event.CodeName()]; ok && event.Type == evdev.EV_KEY {
		switch event.Value {
		case KeyDown:
			logger.Info("modkey [DOWN]")
			tapAndHold.pressedCounter = kbd.downCounter
			kbd.prev = tapAndHold
		case KeyUp:
			logger.Info("modkey [UP]", "kbd.downCounter", kbd.downCounter)
			if tapAndHold.pressedCounter == kbd.downCounter {
				// immediate release, no other keydown events in between, means => TAP behaviour
				logger.Info("dispatching [TAP]", "keycode", evdev.KEYToString[parseKeyCode(kbd.prev.Tap)])
				kbd.dispatchKeyCodes(eventKeyPress(parseKeyCode(tapAndHold.Tap))...)
				kbd.prev = nil
				return
			}
			kbd.dispatchKeyCodes(eventKeyUp(parseKeyCode(tapAndHold.Hold)))
		}

		return
	}

	logger.Info("non-modkey")
	kbd.dispatchKeyCodes(event)
}

func Start(ctx context.Context, keyboard *evdev.InputDevice) error {
	logger.Info("listening on", "keyboard", must(keyboard.Name()))
	defer logger.Info("STOPPED listening on", "keyboard", must(keyboard.Name()))

	clone, err := cloneDevice(keyboard.Path())
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

	c, err := LoadConfig(configFile)
	if err != nil {
		panic(err)
	}

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

var logger *fastlog.Logger

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

	logger = fastlog.New(fastlog.ShowDebugLogs(debug), fastlog.WithoutCaller())

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
