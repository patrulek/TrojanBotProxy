package main

import (
	"github.com/pelletier/go-toml"
)

const (
	defaultConfigFilepath = "config.toml"
)

type Config struct {
	Telegram Telegram `toml:"telegram"`
}

type Telegram struct {
	AppId       int    `toml:"app_id"`
	AppHash     string `toml:"app_hash"`
	PhoneNumber string `toml:"phone_number"`

	TrojanContactName string `toml:"trojan_contact_name"`
}

func LoadConfig(path string) (Config, error) {
	if path == "" {
		path = defaultConfigFilepath
	}

	tree, err := toml.LoadFile(path)
	if err != nil {
		return Config{}, err
	}

	var config Config
	if err := tree.Unmarshal(&config); err != nil {
		return Config{}, err
	}

	return config, nil
}
