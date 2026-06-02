package publish_event

import (
	"time"

	"github.com/gleo/subscribers/common/kafka"
)

type PublishEventConfig struct {
	KafkaWriter kafka.WriterConfig `yaml:"kafka_writer" mapstructure:"kafka_writer"`
	Event       EventConfig        `yaml:"event"        mapstructure:"event"`
}

type EventConfig struct {
	TenantIds   []string           `yaml:"tenant_ids"  mapstructure:"tenant_ids"`
	Subscribers []SubscriberConfig `yaml:"subscribers" mapstructure:"subscribers"`
	Interval    time.Duration      `yaml:"interval"    mapstructure:"interval" default:"1s"`
}

type SubscriberConfig struct {
	SubscriberId string `yaml:"subscriber_id" mapstructure:"subscriber_id"`
	Data         string `yaml:"data"          mapstructure:"data"`
}
