package kafka

import (
	"fmt"

	"go.uber.org/zap"
)

type kafkaLogger struct {
	Logger *zap.SugaredLogger
}

func (r *kafkaLogger) Printf(msg string, params ...interface{}) {
	// Disable verbose kafka logs
	//r.Logger.Debug(fmt.Sprintf(msg, params...))
}

type kafkaErrLogger struct {
	Logger *zap.SugaredLogger
}

func (r *kafkaErrLogger) Printf(msg string, params ...interface{}) {
	r.Logger.Error(fmt.Sprintf(msg, params...))
}
