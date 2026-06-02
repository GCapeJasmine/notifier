package config

import (
	"github.com/gleo/subscribers/common/kafka"
	"github.com/gleo/subscribers/common/postgres"
	"github.com/gleo/subscribers/common/utils"
	"github.com/gleo/subscribers/pkg/partner"
	"github.com/gleo/subscribers/pkg/telemetry"
	"github.com/gleo/subscribers/pkg/workflow"
)

type EventStreamingConfig struct {
	// Datasource
	KafkaReader  kafka.ReaderConfig `yaml:"kafka_reader"  mapstructure:"kafka_reader"`
	PostgresConfig postgres.Config  `yaml:"postgres"      mapstructure:"postgres"`

	// Extension
	Metric               telemetry.ExporterConfig `yaml:"metric"                  mapstructure:"metric"`
	NotifierMaxDuration  workflow.MaxWaitDuration  `yaml:"notifier_max_duration"   mapstructure:"notifier_max_duration"`
	RetryOption          utils.RetryOption         `yaml:"retry_option"            mapstructure:"retry_option"`
	PartnerConfig        partner.Config            `yaml:"partner"       mapstructure:"partner"`
	WorkflowName         string                    `yaml:"workflow_name" mapstructure:"workflow_name"`
}
