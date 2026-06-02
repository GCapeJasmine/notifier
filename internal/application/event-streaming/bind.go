package event_streaming

import (
	"github.com/gleo/subscribers/common/kafka"
	"github.com/gleo/subscribers/common/postgres"
	"github.com/gleo/subscribers/common/utils"
	"github.com/gleo/subscribers/internal/adapter/deadletter"
	"github.com/gleo/subscribers/internal/application/event-streaming/common"
	"github.com/gleo/subscribers/internal/application/event-streaming/config"
	"github.com/gleo/subscribers/internal/application/event-streaming/util"
	"github.com/gleo/subscribers/pkg/partner"
	"github.com/gleo/subscribers/pkg/telemetry"
	"github.com/gleo/subscribers/pkg/workflow"
	"github.com/google/wire"
)

var (
	NotifyWorkflowProviderSet = wire.NewSet(
		NewNotifier,
		NewNotifyWorkflow,
		configProviderSet,
		commonExtensionProviderSet,
		dependencyProviderSet,
	)

	commonExtensionProviderSet = wire.NewSet(
		workflow.NewRetry,
		workflow.NewMaxWait,
		util.NewEventParser,
		util.NewDeadLetterProcessor,
	)

	dependencyProviderSet = wire.NewSet(
		kafka.NewReader,
		workflow.NewSegmentIOSource,
		postgres.NewGormDB,
		deadletter.NewDeadLetterRepository,
		partner.NewPartnerClient,
	)

	configProviderSet = wire.NewSet(
		WireKafkaReaderConfig,
		WireRetryOptionConfig,
		WireMaxWaitConfig,
		WirePartnerConfig,
		WirePostgresConfig,
	)
)

func NewNotifyWorkflow(
	source workflow.EventSource[workflow.KafkaMessage],
	deadLetter *util.DeadLetterProcessor,
	retry *workflow.Retry,
	parser *util.NotifyEventParser,
	maxWait *workflow.MaxWait,
	notifier *Notifier,
) (*workflow.Workflow[workflow.KafkaMessage], error) {
	return workflow.NewWorkflowBuilder[workflow.KafkaMessage]().
		Source(source).
		AddProcessor(deadLetter).
		AddProcessor(retry).
		AddProcessor(maxWait).
		AddProcessor(parser).
		AddProcessor(notifier).
		Name(common.NotifyWorkflowName).
		Build()
}

func WireKafkaReaderConfig(cfg config.EventStreamingConfig) kafka.ReaderConfig {
	return cfg.KafkaReader
}

func WireMetricConfig(cfg config.EventStreamingConfig) telemetry.ExporterConfig {
	return cfg.Metric
}

func WireRetryOptionConfig(cfg config.EventStreamingConfig) utils.RetryOption {
	return cfg.RetryOption
}

func WireMaxWaitConfig(cfg config.EventStreamingConfig) workflow.MaxWaitDuration {
	return cfg.NotifierMaxDuration
}

func WirePartnerConfig(cfg config.EventStreamingConfig) partner.Config {
	return cfg.PartnerConfig
}

func WirePostgresConfig(cfg config.EventStreamingConfig) postgres.Config {
	return cfg.PostgresConfig
}
