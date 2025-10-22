package confighandler

import (
	"fmt"

	"github.com/spf13/viper"
	ctrl "sigs.k8s.io/controller-runtime"
)

const DefaultConfigName = "controller-config"

var setupLog = ctrl.Log.WithName("confighandler")

func LoadConfig() *Config {
	v := viper.New()

	v.SetConfigName(DefaultConfigName)
	v.SetConfigType("yaml")
	v.AddConfigPath("./config")
	v.AddConfigPath(".")

	v.SetEnvPrefix("TAGEMON")
	v.AutomaticEnv()

	_ = v.BindEnv("serviceAccountName", "TAGEMON_SERVICEACCOUNTNAME")
	_ = v.BindEnv("watchNamespace", "TAGEMON_WATCHNAMESPACE")
	_ = v.BindEnv("tagsHandler.viewArn", "TAGEMON_TAGSHANDLER_VIEWARN")
	_ = v.BindEnv("tagsHandler.region", "TAGEMON_TAGSHANDLER_REGION")
	_ = v.BindEnv("tagsHandler.namespace", "TAGEMON_TAGSHANDLER_NAMESPACE")
	_ = v.BindEnv("tagsHandler.interval", "TAGEMON_TAGSHANDLER_INTERVAL")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			setupLog.Info("Config file not found, using environment variables only")
		} else {
			setupLog.Error(err, "Error reading config file - this is a fatal error")
			panic(fmt.Sprintf("Invalid configuration file: %v", err))
		}
	} else {
		setupLog.Info("Loaded config from file", "path", v.ConfigFileUsed())
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		setupLog.Error(err, "Failed to unmarshal config - this is a fatal error")
		panic(fmt.Sprintf("Invalid configuration file: %v", err))
	}

	return &config
}
