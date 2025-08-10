package main

import (
	"fmt"
	"os"
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
	KeyMap []TapAndHold `json:"keymap"`
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
