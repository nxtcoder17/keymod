package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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

func eventToString(ev *evdev.InputEvent) string {
	return fmt.Sprintf("[type] %-12s[key] %s", parseEventType(ev.Value), ev.CodeName())
}

func findKeyboard() (*evdev.InputDevice, error) {
	devicePaths, err := evdev.ListDevicePaths()
	if err != nil {
		fmt.Println("Failed to list input devices:", err)
		return nil, err
	}

	logger.Info("devices", "list", devicePaths)

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
	for i := range events {
		kdb.device.WriteOne(events[i])
	}

	kdb.device.WriteOne(&evdev.InputEvent{
		Type:  evdev.EV_SYN,
		Code:  evdev.SYN_REPORT,
		Value: 0,
	})
}

type ModKey struct {
	OnTap  func() error
	OnHold func() error
}

type MyModKeyboard struct {
	IsHoldingSpace  bool
	IsPressingSpace bool
	SpacePressedAt  time.Time

	IsHoldingCaps  bool
	IsPressingCaps bool
	CapsPressedAt  time.Time

	device  *evdev.InputDevice
	ModKeys map[evdev.EvCode]ModKey

	EventsCh        chan *evdev.InputEvent
	KeyDownEventsCh chan *evdev.InputEvent
}

const thresholdTime = 100

func (kdb *MyModKeyboard) handleSpaceKey(event *evdev.InputEvent) {
	switch event.Value {
	case KeyUp:
		{
			logger.Info("got space [UP]", "event", eventToString(event), "isPressingSpace", kdb.IsPressingSpace, "isHoldingSpace", kdb.IsHoldingSpace)

			if kdb.IsPressingSpace {
				kdb.IsPressingSpace = false
				logger.Info("dispatching space")
				kdb.dispatchKeyCodes(eventKeyPress(evdev.KEY_SPACE)...)
			}

			if kdb.IsHoldingSpace {
				kdb.dispatchKeyCodes(eventKeyUp(evdev.KEY_LEFTSHIFT))
			}

			kdb.IsPressingSpace = false
			kdb.IsHoldingSpace = false
		}
	case KeyDown:
		{
			logger.Info("got space [Down]", "event", eventToString(event))

			kdb.IsPressingSpace = true
			kdb.SpacePressedAt = time.Now()
		}
	case KeyHold:
		{
			kdb.IsHoldingSpace = true
			logger.Debug("got space [HOLD]", "type", event.TypeName(), "code", event.CodeName(), "event", event)
		}
	}
}

func (kdb *MyModKeyboard) handleCapsKey(event *evdev.InputEvent) {
	switch event.Value {
	case KeyUp:
		{
			logger.Info("got caps [UP]", "event", eventToString(event))

			if kdb.IsPressingCaps {
				logger.Info("dispatching Esc")
				kdb.dispatchKeyCodes(eventKeyPress(evdev.KEY_ESC)...)
			}

			if kdb.IsHoldingCaps {
				kdb.dispatchKeyCodes(eventKeyUp(evdev.KEY_LEFTCTRL))
			}

			kdb.IsHoldingCaps = false
			kdb.IsPressingCaps = false
		}
	case KeyDown:
		{
			logger.Info("got Caps [Down]", "event", eventToString(event))
			kdb.IsPressingCaps = true
			kdb.CapsPressedAt = time.Now()
		}
	case KeyHold:
		{
			kdb.IsHoldingCaps = true
			logger.Info("got caps [HOLD]", "event", eventToString(event))
		}
	}
}

func (kdb *MyModKeyboard) handler(event *evdev.InputEvent) {
	if !strings.HasPrefix(event.CodeName(), "KEY_") {
		kdb.dispatchKeyCodes(event)
		return
	}

	switch event.Code {
	case evdev.KEY_SPACE:
		kdb.handleSpaceKey(event)
	case evdev.KEY_CAPSLOCK:
		kdb.handleCapsKey(event)
	default:
		{
			if event.Value != KeyDown {
				kdb.dispatchKeyCodes(event)
				return
			}

			events := []*evdev.InputEvent{event}

			if event.Value == KeyDown && kdb.IsPressingSpace || kdb.IsHoldingSpace {
				kdb.IsHoldingSpace = true
				kdb.IsPressingSpace = false
				logger.Info("holding space key", "event", eventToString(event))
				events = withModifierKey(evdev.KEY_LEFTSHIFT, events...)
			}

			if event.Value == KeyDown && kdb.IsPressingCaps || kdb.IsHoldingCaps {
				kdb.IsHoldingCaps = true
				kdb.IsPressingCaps = false
				logger.Info("holding caps key", "event", eventToString(event))
				events = withModifierKey(evdev.KEY_LEFTCTRL, events...)
			}

			logger.Debug("dispatching", "event", eventToString(event))
			kdb.dispatchKeyCodes(events...)
		}
	}
}

var logger *fastlog.Logger

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "--debug")
	flag.Parse()

	logger = fastlog.New(fastlog.Options{
		Writer:        os.Stderr,
		ShowCaller:    true,
		ShowDebugLogs: debug,
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
		logger.Error("failed to clone device", "err", err)
		os.Exit(1)
	}

	mykb := &MyModKeyboard{device: clone, IsHoldingSpace: false, IsHoldingCaps: false, EventsCh: make(chan *evdev.InputEvent, 1)}

	go func() {
		for ev := range mykb.EventsCh {
			if strings.HasPrefix(ev.CodeName(), "KEY_") {
				logger.Debug("keyboard input", "event", eventToString(ev))
			}
		}
	}()

	for ctx.Err() == nil {
		ev, err := keyboard.ReadOne()
		if err != nil {
			logger.Error("while reading event from source keyboard", "err", err)
			<-time.After(3 * time.Second)
			continue
		}

		mykb.EventsCh <- ev
		mykb.handler(ev)
	}
}
