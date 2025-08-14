/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package confighandler

import (
	"github.com/spf13/viper"
	ctrl "sigs.k8s.io/controller-runtime"
)

const DefaultConfigName = "controller-config"

var setupLog = ctrl.Log.WithName("confighandler")

// LoadConfig loads configuration from file and environment variables
func LoadConfig() *Config {
	v := viper.New()

	// Configure Viper - NO DEFAULTS
	v.SetConfigName(DefaultConfigName)
	v.SetConfigType("yaml")
	v.AddConfigPath("./config")
	v.AddConfigPath(".")

	// Enable environment variable support
	v.SetEnvPrefix("TAGEMON")
	v.AutomaticEnv()

	// Try to read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			setupLog.Info("Config file not found, using environment variables only")
		} else {
			setupLog.Error(err, "Error reading config file")
		}
	} else {
		setupLog.Info("Loaded config from file", "path", v.ConfigFileUsed())
	}

	// Unmarshal into struct
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		setupLog.Error(err, "Failed to unmarshal config")
		return &Config{}
	}

	return &config
}
