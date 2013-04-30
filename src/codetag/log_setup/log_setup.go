package log_setup

import (
	"fmt"
	"github.com/vaughan0/go-logging"
	"github.com/kylelemons/go-gypsy/yaml"
)


type YAMLConfig yaml.Map

func (config YAMLConfig) LoggerSettings() (loggers map[string]string) {
	section, ok := config["loggers"]
	if !ok {
		panic(fmt.Errorf("'loggers' section is must be a map"))
	}
	section_map, ok := section.(yaml.Map)
	if !ok {
		panic(fmt.Errorf("'loggers' section is missing"))
	}

	loggers = map[string]string{}
	for k, node := range section_map {
		v, ok := node.(yaml.Scalar)
		if !ok {
			panic(fmt.Errorf("Logging level must be string: %v", v))
		}
		loggers[k] = string(v)
	}

	return
}

func (config YAMLConfig) Plugins() (plugins []logging.PluginConfig) {
	var (
		key string
		options map[string]string
	)
	for key, section := range config {
		if key == "loggers" {
			continue
		}
		section_map, ok := section.(yaml.Map)
		if !ok {
			panic(fmt.Errorf("Failed to init Outputter: %s", key))
		}
		options = map[string]string{}
		for k, node := range section_map {
			v, ok := node.(yaml.Scalar)
			if !ok {
				panic(fmt.Errorf("Invalid type (must be string, key: %q): %v", k, node))
			}
			options[k] = string(v)
		}
	}
		plugins = append(plugins, logging.PluginConfig{
			Name: key,
			Options: options,
		})
	return
}


// Configures the logging hierarchy from a YAML object
func SetupYAML(cp *yaml.Map) (err error) {
	var config_map interface{}
	config_map = *cp
	config, ok := config_map.(YAMLConfig)
	if !ok {
		return fmt.Errorf("Failed to process logging configuration: %s", cp)
	}

	defer func() {
		panic_err := recover()
		if err == nil && panic_err != nil {
			err, ok = panic_err.(error)
		}
	}()

	logging.SetupConfig(config)
	return
}
