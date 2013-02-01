package log_setup

import (
	"fmt"
	"strings"
	"github.com/mk-fg/go-logging"
	"github.com/kylelemons/go-gypsy/yaml"
)


// Loads the appropriate plugin and creates an outputter, given a configuration section.
func newOutputterConfig(cp *yaml.Map) (logging.Outputter, error) {
	config := *cp

	// Convert yaml.Map to map[string]string{}
	config_map := map[string]string{}
	for k, node := range config {
		v, ok := node.(yaml.Scalar)
		if !ok {
			return nil, fmt.Errorf("Invalid type: %v", node)
		}
		config_map[k] = string(v)
	}

	// Get/create plugin/outputter from the "type" option
	name, ok := config_map["type"]
	if !ok {
		return nil, fmt.Errorf("Plugin name not specified: %v", config_map)
	}
	plugin, err := logging.GetOutputPlugin(name)
	if plugin == nil {
		return nil, err
	}
	output, err := plugin.CreateOutputter(config_map)
	if err != nil {
		return nil, err
	}

	// Check for the "threshold" option
	thresh, ok := config_map["threshold"]
	if ok {
		level, ok := logging.ReverseLevelStrings[strings.ToUpper(thresh)]
		if ok {
			output = logging.ThresholdOutputter{level, output}
		} else {
			return nil, fmt.Errorf("Invalid threshold: %v", thresh)
		}
	}

	return output, nil
}


// Configures the logging hierarchy from a YAML object
func SetupYAML(cp *yaml.Map) (err error) {
	config := *cp
	// fmt.Println(config)

	var (
		section yaml.Node
		section_map yaml.Map
	)

	// Create outputters
	outputters := make(map[string]logging.Outputter)
	for key, section := range config {
		if key != "loggers" {
			section_map, ok := section.(yaml.Map)
			if ok {
				output, err := newOutputterConfig(&section_map)
				if err == nil {
					outputters[key] = output
					continue
				}
			}
			return fmt.Errorf("Failed to init Outputter: %s", key)
		}
	}

	section, ok := config["loggers"]
	if !ok {
		return fmt.Errorf("'loggers' section is must be a map")
	}
	section_map, ok = section.(yaml.Map)
	if !ok {
		return fmt.Errorf("'loggers' section is missing")
	}

	// Setup loggers
	for name, node := range section_map {
		v, ok := node.(yaml.Scalar)
		if !ok {
			return fmt.Errorf("Logging level must be string: %v", v)
		}
		parts := strings.Split(string(v), ",")
		level, ok := logging.ReverseLevelStrings[strings.ToUpper(parts[0])]
		if !ok {
			return fmt.Errorf("Unknown logging level: %v", v)
		}
		// Get the logger by its name, treating "root" as a special name
		var logger *logging.Logger
		if name == "root" {
			logger = logging.Root
		} else {
			logger = logging.Get(name)
		}
		logger.Threshold = level
		// Handle extra options
		for _, outputKey := range parts[1:] {
			if outputKey == "nopropagate" {
				logger.NoPropagate = true
			} else {
				// Assign an outputter
				if outputter := outputters[outputKey]; outputter != nil {
					logger.AddOutput(outputter)
				} else {
					return fmt.Errorf("Unknown logging output: %v", outputKey)
				}
			}
		}
	}

	logging.Root.Configure()
	logging.CustomSetup()
	return nil
}
