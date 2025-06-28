package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

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

func findKeyboard() (*evdev.InputDevice, error) {
	devicePaths, err := evdev.ListDevicePaths()
	if err != nil {
		fmt.Println("Failed to list input devices:", err)
		return nil, err
	}

	logger.Debug("devices", "list", devicePaths)

	for _, devpath := range devicePaths {
		if devpath.Name == "BY Tech Gaming Keyboard" {
			// if devpath.Name == "CX 2.4G Wireless Receiver Keyboard" {
			dev, err := evdev.Open(devpath.Path)
			if err != nil {
				return nil, err
			}

			return dev, nil
		}

		// dev, err := evdev.Open(devpath.Path)
		// if err != nil {
		// 	return nil, err
		// }
		//
		// if len(dev.CapableEvents(evdev.EV_KEY)) > 0 {
		// 	return dev, nil
		// }
	}

	return nil, fmt.Errorf("failed to find a keyboard device")
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
	// defer clonedDev.Close()
	// moveMouse(clonedDev)
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

	// mods = append(mods, &evdev.InputEvent{
	// 	Type: evdev.EV_KEY, Code: modifier, Value: KeyUp,
	// })

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
	device    *evdev.InputDevice
	keyDownCh chan *evdev.InputEvent
	cfg       *ParsedConfig

	onModHold    func()
	onModRelease func()

	sendHoldEvent context.CancelFunc
	counter       int
	downCounter   int
	prev          *TapAndHold
}

func (kbd *MyModKeyboard) handler(event *evdev.InputEvent) {
	if !strings.HasPrefix(event.CodeName(), "KEY_") {
		kbd.dispatchKeyCodes(event)
		return
	}

	kbd.counter += 1
	if isKeyDown(event) {
		kbd.downCounter += 1
	}

	logger := logger.With("event", eventToString(event), "kbd.prev", kbd.prev == nil, "kbd.counter", kbd.counter, "kbd.downCounter", kbd.downCounter)

	if kbd.prev != nil && kbd.prev.pressedIdx+1 == kbd.downCounter && isKeyDown(event) {
		logger.Info("dispatching [HOLD]", "keycode", evdev.KEYToString[parseKeyCode(kbd.prev.Hold)])
		kbd.dispatchKeyCodes(eventKeyDown(parseKeyCode(kbd.prev.Hold)))
		kbd.prev = nil
	}

	if tapAndHold, ok := kbd.cfg.ModMap[event.CodeName()]; ok && event.Type == evdev.EV_KEY {
		switch event.Value {
		case KeyDown:
			logger.Info("modkey [DOWN]")
			tapAndHold.pressedIdx = kbd.downCounter
			kbd.prev = tapAndHold
		case KeyUp:
			logger.Info("modkey [UP]", "kbd.downCounter", kbd.downCounter)
			if tapAndHold.pressedIdx == kbd.downCounter {
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

var logger *fastlog.Logger

func filter[T any](arr []T, fn func(T) bool) []T {
	result := make([]T, len(arr))
	for i := range arr {
		if fn(arr[i]) {
			result = append(result, arr[i])
		}
	}
	return result
}

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "--debug")
	flag.Parse()

	logger = fastlog.New(fastlog.Options{
		Writer:        os.Stderr,
		ShowCaller:    false,
		ShowDebugLogs: debug,
		ShowTimestamp: true,
		EnableColors:  true,
		Format:        fastlog.ConsoleFormat,
	})

	// var keyboard *evdev.InputDevice
	// for _, dev := range devices {
	// 	if dev.Capabilities[evdev.EV_KEY] != nil {
	// 		keyboard = dev
	// 		break
	// 	}
	// }

	keyboard, err := findKeyboard()
	if err != nil {
		logger.Error("failed to find keyboard", "err", err)
		os.Exit(1)
	}

	logger.Info("listening on", "keyboard", must(keyboard.Name()))

	clone, err := cloneDevice(keyboard.Path())
	if err != nil {
		logger.Error("failed to clone device", "err", err)
		os.Exit(1)
	}
	defer clone.Close()

	ctx, cf := signal.NotifyContext(context.TODO(), syscall.SIGINT, syscall.SIGTERM)
	defer cf()

	if err := keyboard.Grab(); err != nil {
		logger.Error("failed to grab original keyboard", "err", err)
		os.Exit(1)
	}
	eventsCh := make(chan *evdev.InputEvent, 1)

	c, err := LoadConfig()
	if err != nil {
		panic(err)
	}

	logger.Info("hello", "cfg.modmap", c.ModMap)

	mykb := &MyModKeyboard{
		device:    clone,
		keyDownCh: make(chan *evdev.InputEvent, 1),
		cfg:       c,
	}

	go func() {
		for ev := range eventsCh {
			if strings.HasPrefix(ev.CodeName(), "KEY_") {
				// logger.Debug("keyboard input", "event", eventToString(ev))
			}
		}
	}()

	// go func() {
	// 	for ev := range mykb.keyDownCh {
	// 		logger.Debug("keyboard input [KEYDOWN]", "event", eventToString(ev))
	// 	}
	// }()

	go func() {
		<-mykb.keyDownCh
	}()

	for ctx.Err() == nil {
		ev, err := keyboard.ReadOne()
		if err != nil {
			logger.Error("while reading event from source keyboard", "err", err)
			return
		}

		eventsCh <- ev
		// if strings.HasPrefix(ev.CodeName(), "KEY_") && ev.Value == KeyDown {
		// 	mykb.keyDownCh <- ev
		// }
		mykb.handler(ev)
	}
}
