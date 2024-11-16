package config

import (
	"github.com/pelletier/go-toml"
)

const (
	defaultConfigFilepath = "config.toml"
)

type Config struct {
	Telegram   Telegram   `toml:"telegram"`
	DataSource DataSource `toml:"datasource"`
}

type Telegram struct {
	AppId       int    `toml:"app_id"`
	AppHash     string `toml:"app_hash"`
	PhoneNumber string `toml:"phone_number"`

	TrojanContactName string `toml:"trojan_contact_name"`
}

type DataSource struct {
	Host      string `toml:"host"`
	Port      int    `toml:"port"`
	Method    string `toml:"method"`
	Auth      Auth   `toml:"auth"`
	Interval  string `toml:"interval"`
	Params    Params `toml:"params"`
	TokenPath string `toml:"token_path"`
}

type Auth struct {
	Context string `toml:"context"`
	Name    string `toml:"name"`
	Value   string `toml:"value"`
}

type Params map[string]string

func Load(path string) (Config, error) {
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
