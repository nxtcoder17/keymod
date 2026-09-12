package main

import (
	"fmt"
	"sync"

	evdev "github.com/holoplot/go-evdev"
)

// MockInputDevice is a mock implementation of InputDevice for testing
type MockInputDevice struct {
	mu            sync.Mutex
	writtenEvents []*evdev.InputEvent
	readEvents    []*evdev.InputEvent
	readIndex     int
	name          string
	path          string
	grabCalled    bool
	closeCalled   bool
}

func NewMockInputDevice(name, path string) *MockInputDevice {
	return &MockInputDevice{
		name:          name,
		path:          path,
		writtenEvents: make([]*evdev.InputEvent, 0),
		readEvents:    make([]*evdev.InputEvent, 0),
	}
}

func (m *MockInputDevice) WriteOne(event *evdev.InputEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writtenEvents = append(m.writtenEvents, event)
	return nil
}

func (m *MockInputDevice) ReadOne() (*evdev.InputEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.readIndex >= len(m.readEvents) {
		return nil, fmt.Errorf("no more events to read")
	}

	event := m.readEvents[m.readIndex]
	m.readIndex++
	return event, nil
}

func (m *MockInputDevice) Grab() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.grabCalled = true
	return nil
}

func (m *MockInputDevice) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return nil
}

func (m *MockInputDevice) Name() (string, error) {
	return m.name, nil
}

func (m *MockInputDevice) Path() string {
	return m.path
}

// Test helper methods

func (m *MockInputDevice) AddReadEvent(event *evdev.InputEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readEvents = append(m.readEvents, event)
}

func (m *MockInputDevice) GetWrittenEvents() []*evdev.InputEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	events := make([]*evdev.InputEvent, len(m.writtenEvents))
	copy(events, m.writtenEvents)
	return events
}

func (m *MockInputDevice) ClearWrittenEvents() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writtenEvents = m.writtenEvents[:0]
}

func (m *MockInputDevice) WasGrabbed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.grabCalled
}

func (m *MockInputDevice) WasClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closeCalled
}
