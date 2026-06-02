package main

import (
	"context"

	dead_letter_replay "github.com/gleo/subscribers/internal/application/job/dead-letter-replay"
	"github.com/gleo/subscribers/internal/application/job/dead-letter-replay/wire"
	"github.com/gleo/subscribers/pkg/utils"
)

func main() {
	utils.RunMain[dead_letter_replay.DeadLetterReplayConfig](
		"dead-letter-replay",
		func(cfg dead_letter_replay.DeadLetterReplayConfig) error {
			job, err := wire.WireReplayWorkflow(cfg)
			if err != nil {
				return err
			}
			_, err = job.Run(context.Background())
			return err
		})
}
