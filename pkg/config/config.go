package config

import (
	"bufio"
	"io"
	"os"
	"strings"

	"github.com/alapierre/itrust-updater/pkg/logging"
)

var logger = logging.Component("pkg/config")

type Config map[string]string

func Parse(r io.Reader) (Config, error) {
	config := make(Config)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		config[key] = value
	}
	return config, scanner.Err()
}

func LoadFile(path string) (Config, error) {
	logger.Debugf("Loading config from %s", path)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(Config), nil
		}
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

func (c Config) Merge(other Config) {
	for k, v := range other {
		c[k] = v
	}
}

func (c Config) Get(key string, defaultValue string) string {
	if v, ok := c[key]; ok {
		return v
	}
	return defaultValue
}

func MergeConfigs(priority ...Config) Config {
	res := make(Config)
	for i := len(priority) - 1; i >= 0; i-- {
		res.Merge(priority[i])
	}
	return res
}

func GetEnvConfig() Config {
	res := make(Config)
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "ITRUST_") {
			parts := strings.SplitN(env, "=", 2)
			res[parts[0]] = parts[1]
		}
	}
	return res
}
