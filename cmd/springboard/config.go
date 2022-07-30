package main

import (
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

type configYaml struct {
	Federates     []string
	Port          uint
	FQDN          string
	PropagateWait time.Duration `yaml:"propagate_wait"`
	AdminBoard    string        `yaml:"admin_board"`
}

type Config struct {
	yaml configYaml
}

func ConfigFromFile(path string) (config Config, err error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		err = errors.Wrapf(err, "Could not read file %s", path)
		return
	}
	var rawConfig configYaml
	err = yaml.Unmarshal(data, &rawConfig)
	config.yaml = rawConfig
	if err != nil {
		err = errors.Wrap(err, "Could not unmarshal config yaml")
	}
	return
}

func (config Config) Federates() []string {
	fromEnv, inEnv := os.LookupEnv("SB_FEDERATES")
	if inEnv {
		return strings.Split(fromEnv, ",")
	}
	return config.yaml.Federates
}

func (config Config) Port() uint {
	envPort, err := strconv.ParseUint(os.Getenv("PORT"), 10, 16)
	if err != nil && envPort != 0 {
		return uint(envPort)
	} else if config.yaml.Port != 0 {
		return config.yaml.Port
	} else {
		return 8000
	}
}

func (config Config) FQDN() string {
	fromEnv, inEnv := os.LookupEnv("SB_FQDN")
	if inEnv {
		return fromEnv
	}
	if config.yaml.FQDN != "" {
		return config.yaml.FQDN
	}
	hostname, err := os.Hostname()
	if err != nil {
		return hostname
	} else {
		return "localhost"
	}
}

func (config Config) PropagateWait() time.Duration {
	fromEnv, inEnv := os.LookupEnv("SB_PROPAGATE_WAIT")
	if inEnv {
		duration, err := time.ParseDuration(fromEnv)
		if err != nil {
			panic(err)
		}
		return duration
	}
	if config.yaml.PropagateWait == 0 {
		return time.Duration(5 * time.Minute)
	} else {
		return config.yaml.PropagateWait
	}
}

func (config Config) AdminBoard() string {
	fromEnv, inEnv := os.LookupEnv("SB_ADMIN_BOARD")
	if inEnv {
		return fromEnv
	}
	return config.yaml.AdminBoard
}
