package main

import (
	"context"

	"github.com/gleo/subscribers/common/log"
	"github.com/gleo/subscribers/internal/application/event-streaming/config"
	"github.com/gleo/subscribers/internal/application/event-streaming/wire"
	"github.com/gleo/subscribers/pkg/utils"
	"go.uber.org/zap"
)

func main() {
	utils.RunMain[config.EventStreamingConfig]("webhook-notifier", func(cfg config.EventStreamingConfig) error {
		exporter := wire.WireExporter(cfg)
		go func() {
			err := exporter.Start()
			if err != nil {
				log.Logger.Fatal("cannot start metric exporter", zap.Error(err))
			}
		}()

		p, err := wire.GetWorkflow(cfg.WorkflowName, cfg)
		if err != nil {
			log.Logger.Fatal("cannot init workflow", zap.Error(err))
		}
		_, err = p.Run(context.Background())
		return err
	})
}
