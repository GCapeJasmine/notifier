package dead_letter_replay

import (
	"time"

	"github.com/gleo/subscribers/internal/application/event-streaming/config"
)

type DeadLetterReplayConfig struct {
	Replay         ReplayConfig                `yaml:"replay" mapstructure:"replay"`
	EventStreaming config.EventStreamingConfig `yaml:"event_streaming" mapstructure:"event_streaming"`
}

type ReplayConfig struct {
	Ids      []int64   `yaml:"ids" mapstructure:"ids"`
	FromId   int64     `yaml:"from_id" mapstructure:"from_id"`
	ToId     int64     `yaml:"to_id" mapstructure:"to_id"`
	TenantId string    `yaml:"tenant_id" mapstructure:"tenant_id"`
	From     time.Time `yaml:"from" mapstructure:"from"`
	To       time.Time `yaml:"to" mapstructure:"to"`
}
