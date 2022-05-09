package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"

	"github.com/golang/glog"
)

type DeviceMatch struct {
	Match  []string
	Group  string `yaml:",omitempty"`
	Search string `yaml:",omitempty"`
}

type Config struct {
	Matchers      []DeviceMatch `yaml:",flow"`
	DefaultSearch string        `yaml:"search"`
}

func ParseConfig(path string) (cfg Config, err error) {
	_, err = os.Stat(path)
	if err != nil {
		return Config{}, err
	}

	yamlFile, err := os.ReadFile(path)
	if err != nil {
		glog.Error(err)
		return Config{}, err
	}

	err = yaml.Unmarshal(yamlFile, &cfg)
	if err != nil {
		glog.Error(err)
		return Config{}, err
	}

	for idx, devMatch := range cfg.Matchers {
		if len(devMatch.Search) == 0 {
			cfg.Matchers[idx].Search = cfg.DefaultSearch
		}
	}

	return cfg, nil
}

func (cfg *Config) String() string {
	str := fmt.Sprintln("devices:")
	for _, match := range cfg.Matchers {
		str += fmt.Sprintf("- match: %s\n", match.Match)
		str += fmt.Sprintf("  group: %s\n", match.Group)
		str += fmt.Sprintf("  search: %s\n", match.Search)
	}

	return str
}
