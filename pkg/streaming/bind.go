package streaming

import (
	"github.com/gleo/subscribers/common/kafka"
	"github.com/google/wire"
)

var (
	KafkaProviderSet = wire.NewSet(
		kafka.NewReader,
		kafka.NewWriter,
	)
)
