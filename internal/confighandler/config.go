package confighandler

import "time"

type Config struct {
	ServiceAccountName string            `mapstructure:"serviceAccountName"`
	WatchNamespace     string            `mapstructure:"watchNamespace"`
	TagsHandler        TagsHandlerConfig `mapstructure:"tagsHandler"`
	Yace               YaceConfig        `mapstructure:"yace"`
}

type TagsHandlerConfig struct {
	ViewARN                        string            `mapstructure:"viewArn"`
	Region                         string            `mapstructure:"region"`
	Namespace                      string            `mapstructure:"namespace"`
	Interval                       *time.Duration    `mapstructure:"interval"`
	NonCompliantMetricCustomLabels map[string]string `mapstructure:"nonCompliantMetricCustomLabels"`
}

type YaceConfig struct {
	CustomImage string `mapstructure:"customImage"`
}
