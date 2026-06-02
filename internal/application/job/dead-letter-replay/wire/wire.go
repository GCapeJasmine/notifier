//go:build wireinject

package wire

import (
	dead_letter_replay "github.com/gleo/subscribers/internal/application/job/dead-letter-replay"
	"github.com/gleo/subscribers/internal/domain"
	"github.com/gleo/subscribers/pkg/workflow"
	gwire "github.com/google/wire"
)

func InitReplayWorkflow(cfg dead_letter_replay.DeadLetterReplayConfig) (*workflow.Workflow[domain.DeadLetter], error) {
	gwire.Build(dead_letter_replay.DeadLetterReplayProviderSet)
	return nil, nil
}
