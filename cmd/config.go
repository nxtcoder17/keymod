package main

import (
	"fmt"
	"os"
	"strings"

	evdev "github.com/holoplot/go-evdev"
	"github.com/pelletier/go-toml/v2"
)

type TapAndHold struct {
	Key  string `json:"key"`
	Tap  string `json:"tap"`
	Hold string `json:"hold"`

	pressedCounter int
}

type Config struct {
	KeyMap []TapAndHold `json:"keymap"`
}

func normalizeKeyCode(s string) string {
	if !strings.HasPrefix(s, "KEY_") {
		return "KEY_" + s
	}
	return s
}

func LoadConfig(file string) (*ParsedConfig, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}

	logger.Info("LOADED", "config", fmt.Sprintf("%+v\n", cfg))

	pc := &ParsedConfig{
		ModMap: make(map[string]*TapAndHold, len(cfg.KeyMap)),
	}

	for _, th := range cfg.KeyMap {
		th.Key = normalizeKeyCode(th.Key)
		th.Tap = normalizeKeyCode(th.Tap)
		th.Hold = normalizeKeyCode(th.Hold)

		for _, code := range []string{th.Key, th.Tap, th.Hold} {
			if _, ok := evdev.KEYFromString[code]; !ok {
				return nil, fmt.Errorf("unknown key code %q in keymap entry %+v", code, th)
			}
		}

		// Copy per entry so each ModMap value owns its state; the
		// handler must still never mutate these (it copies again per
		// press into pendingPress).
		entry := th
		pc.ModMap[th.Key] = &entry
	}

	return pc, nil
}

type ParsedConfig struct {
	ModMap map[string]*TapAndHold
}
