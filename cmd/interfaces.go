package main

import evdev "github.com/holoplot/go-evdev"

// InputDevice interface for mocking evdev.InputDevice
type InputDevice interface {
	WriteOne(event *evdev.InputEvent) error
	ReadOne() (*evdev.InputEvent, error)
	Grab() error
	Close() error
	Name() (string, error)
	Path() string
}

// inputDeviceWrapper wraps evdev.InputDevice to implement our interface
type inputDeviceWrapper struct {
	*evdev.InputDevice
}

func (w *inputDeviceWrapper) Path() string {
	return w.InputDevice.Path()
}

func (w *inputDeviceWrapper) Name() (string, error) {
	name, err := w.InputDevice.Name()
	return name, err
}
