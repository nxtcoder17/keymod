package main

import (
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type TapAndHold struct {
	Key  string `json:"key"`
	Tap  string `json:"tap"`
	Hold string `json:"hold"`

	pressedCounter int
}

type Config struct {
	ModMap []TapAndHold `json:"modmap"`
}

func LoadConfig() (*ParsedConfig, error) {
	b := []byte(`
[[modmap]]
key = "CAPSLOCK"
tap = "ESC"
hold = "LEFTCTRL"

[[modmap]]
key = "SPACE"
tap = "SPACE"
hold = "LEFTSHIFT"
`)

	var cfg Config
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}

	pc := &ParsedConfig{
		ModMap: make(map[string]*TapAndHold, len(cfg.ModMap)),
	}

	for _, th := range cfg.ModMap {
		if !strings.HasPrefix(th.Key, "KEY_") {
			th.Key = "KEY_" + th.Key
		}
		pc.ModMap[th.Key] = &th
	}

	return pc, nil
}

type ParsedConfig struct {
	ModMap map[string]*TapAndHold
}
