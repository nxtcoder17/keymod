package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	evdev "github.com/holoplot/go-evdev"
	"github.com/pelletier/go-toml/v2"
)

type TapAndHold struct {
	Key        string `json:"key" toml:"key"`
	Tap        string `json:"tap" toml:"tap"`
	Hold       string `json:"hold" toml:"hold"`
	TapTimeout int    `json:"tap-timeout" toml:"tap-timeout"`
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
		ModMap: make(map[evdev.EvCode]*ParsedTapAndHold, len(cfg.KeyMap)),
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

		pc.ModMap[evdev.KEYFromString[th.Key]] = &ParsedTapAndHold{
			Tap:        evdev.KEYFromString[th.Tap],
			Hold:       evdev.KEYFromString[th.Hold],
			TapTimeout: time.Duration(th.TapTimeout) * time.Millisecond,
		}
	}

	return pc, nil
}

type ParsedTapAndHold struct {
	Tap        evdev.EvCode
	Hold       evdev.EvCode
	TapTimeout time.Duration
}

type ParsedConfig struct {
	ModMap map[evdev.EvCode]*ParsedTapAndHold
}
