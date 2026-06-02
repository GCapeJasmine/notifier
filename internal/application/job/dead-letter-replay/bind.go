package dead_letter_replay

import (
	"github.com/gleo/subscribers/common/postgres"
	"github.com/gleo/subscribers/internal/adapter/deadletter"
	"github.com/gleo/subscribers/internal/application/event-streaming/common"
	"github.com/gleo/subscribers/internal/application/event-streaming/config"
	event_streaming_wire "github.com/gleo/subscribers/internal/application/event-streaming/wire"
	"github.com/gleo/subscribers/internal/domain"
	"github.com/gleo/subscribers/pkg/workflow"
	"github.com/google/wire"
)

var (
	DeadLetterReplayProviderSet = wire.NewSet(
		NewDeadLetterSource,
		NewDeadLetterDispatcher,
		NewReplayWorkflow,
		NewWorkflowSuppliers,
		WireReplayConfig,
		WireEventStreamingConfig,
		WirePostgresConfig,
		postgres.NewGormDB,
		deadletter.NewDeadLetterRepository,
	)
)

func WireReplayConfig(cfg DeadLetterReplayConfig) ReplayConfig {
	return cfg.Replay
}

func WireEventStreamingConfig(cfg DeadLetterReplayConfig) config.EventStreamingConfig {
	return cfg.EventStreaming
}

func WirePostgresConfig(cfg DeadLetterReplayConfig) postgres.Config {
	return cfg.EventStreaming.PostgresConfig
}

func NewWorkflowSuppliers() map[string]WorkflowSupplier {
	return map[string]WorkflowSupplier{
		common.NotifyWorkflowName: event_streaming_wire.WireNotifyWorkflow,
	}
}

func NewReplayWorkflow(
	source workflow.EventSource[domain.DeadLetter],
	dispatcher *DeadLetterDispatcher,
) (*workflow.Workflow[domain.DeadLetter], error) {
	return workflow.NewWorkflowBuilder[domain.DeadLetter]().
		Source(source).
		AddProcessor(dispatcher).
		Name("dead-letter-replay").
		Build()
}
