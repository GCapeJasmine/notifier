package utils

import (
	"time"

	"github.com/cenkalti/backoff/v4"
	"go.uber.org/zap"

	"github.com/gleo/subscribers/common/log"
)

type RetryOption struct {
	Enabled             bool          `yaml:"enabled" mapstructure:"enabled"`
	EnabledLog          bool          `yaml:"enabled_log" mapstructure:"enabled_log"`
	InitialInterval     time.Duration `yaml:"initial_interval" mapstructure:"initial_interval"`
	RandomizationFactor float64       `yaml:"randomization_factor" mapstructure:"randomization_factor"`
	Multiplier          float64       `yaml:"multiplier" mapstructure:"multiplier"`
	MaxInterval         time.Duration `yaml:"max_interval" mapstructure:"max_interval"`
	MaxElapsedTime      time.Duration `yaml:"max_elapsed_time" mapstructure:"max_elapsed_time"`
	MaxRetries          uint64        `yaml:"max_retries" mapstructure:"max_retries"`
}

func (r RetryOption) toBackoff() backoff.BackOff {
	b := &backoff.ExponentialBackOff{
		InitialInterval:     r.InitialInterval,
		RandomizationFactor: r.RandomizationFactor,
		Multiplier:          r.Multiplier,
		MaxInterval:         r.MaxInterval,
		MaxElapsedTime:      r.MaxElapsedTime,
		Stop:                backoff.Stop,
		Clock:               backoff.SystemClock,
	}

	if r.MaxRetries > 0 {
		return backoff.WithMaxRetries(b, r.MaxRetries)
	}
	return b
}

func ProcessWithRetryOption(process func() error, retryOption RetryOption) error {
	if retryOption.Enabled {
		exponentialBackOff := retryOption.toBackoff()
		exponentialBackOff.Reset()

		return backoff.Retry(func() error {
			err := process()
			if err != nil && retryOption.EnabledLog {
				log.Logger.Errorw("retry attempt failed", zap.Error(err))
			}
			return err
		}, exponentialBackOff)
	}
	return process()
}

func ProcessWithDataRetryOption[T any](process func() (T, error), retryOption RetryOption) (T, error) {
	if retryOption.Enabled {
		exponentialBackOff := retryOption.toBackoff()
		exponentialBackOff.Reset()

		return backoff.RetryWithData(func() (T, error) {
			t, err := process()
			if err != nil && retryOption.EnabledLog {
				log.Logger.Errorw("retry with data attempt failed", zap.Error(err))
			}
			return t, err
		}, exponentialBackOff)
	}
	return process()
}
