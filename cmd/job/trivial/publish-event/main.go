package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	publish_event "github.com/gleo/subscribers/internal/application/job/publish-event"
	"github.com/gleo/subscribers/pkg/utils"
)

func main() {
	utils.RunMain[publish_event.PublishEventConfig]("publish-event", func(cfg publish_event.PublishEventConfig) error {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return publish_event.NewPublisher(cfg).Run(ctx)
	})
}
