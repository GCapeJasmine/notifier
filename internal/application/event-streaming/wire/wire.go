//go:build wireinject

package wire

import (
	"fmt"

	event_streaming "github.com/gleo/subscribers/internal/application/event-streaming"
	"github.com/gleo/subscribers/internal/application/event-streaming/common"
	"github.com/gleo/subscribers/internal/application/event-streaming/config"
	"github.com/gleo/subscribers/pkg/telemetry"
	"github.com/gleo/subscribers/pkg/workflow"
	gwire "github.com/google/wire"
)

func WireExporter(cfg config.EventStreamingConfig) *telemetry.Exporter {
	exporterConfig := event_streaming.WireMetricConfig(cfg)
	return telemetry.NewExporter(exporterConfig)
}

func GetWorkflow(name string, cfg config.EventStreamingConfig) (*workflow.Workflow[workflow.KafkaMessage], error) {
	switch name {
	case common.NotifyWorkflowName:
		return WireNotifyWorkflow(cfg)
	default:
		return nil, fmt.Errorf("wire: unknown workflow %q", name)
	}
}

func WireNotifyWorkflow(cfg config.EventStreamingConfig) (*workflow.Workflow[workflow.KafkaMessage], error) {
	gwire.Build(event_streaming.NotifyWorkflowProviderSet)
	return nil, nil
}
