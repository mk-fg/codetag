package log_setup

import (
	"fmt"
	"github.com/vaughan0/go-logging"
	"github.com/kylelemons/go-gypsy/yaml"
)


func YAMLMapToStrings(yaml_map yaml.Map) (strings map[string]string) {
	strings = map[string]string{}
	for k, node := range yaml_map {
		v, ok := node.(yaml.Scalar)
		if !ok {
			panic(fmt.Errorf("Expecting scalar value (key: %q): %v", k, node))
		}
		strings[k] = string(v)
	}
	return
}


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
	return YAMLMapToStrings(section_map)
}

func (config YAMLConfig) Plugins() (plugins []logging.PluginConfig) {
	for key, section := range config {
		if key == "loggers" {
			continue
		}
		section_map, ok := section.(yaml.Map)
		if !ok {
			panic(fmt.Errorf("Failed to init Outputter: %s", key))
		}
		plugin := new(logging.PluginConfig)
		plugin.Name, plugin.Options = key, YAMLMapToStrings(section_map)
		plugins = append(plugins, *plugin)
	}
	return
}


// Configures the logging hierarchy from a YAML object
func SetupYAML(yaml_config yaml.Map) (err error) {
	defer func() {
		panic_err := recover()
		if err == nil && panic_err != nil {
			err = panic_err.(error)
		}
	}()
	logging.SetupConfig(YAMLConfig(yaml_config))
	return
}
