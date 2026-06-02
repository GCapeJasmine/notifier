package client

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/samber/lo"

	"github.com/gleo/subscribers/common/log"
)

var (
	schemaAllows     = []string{"http", "https"}
	jsonContainType  = "application/json"
	errInvalidSchema = errors.New("invalid http schema")
)

type RestyConfig struct {
	RetryMaxWait  time.Duration `yaml:"retry_max_wait" mapstructure:"retry_max_wait" default:"30s"`
	RetryInterval time.Duration `yaml:"retry_interval" mapstructure:"retry_interval" default:"500ms"`
	RetryCount    int           `yaml:"retry_count" mapstructure:"retry_count" default:"10"`
	EnableLogger  bool          `yaml:"enable_logger" mapstructure:"enable_logger"`
}

func NewRestyClient(config RestyConfig) *resty.Client {
	client := resty.New()
	if config.EnableLogger {
		client.SetLogger(log.Logger.Named("resty-client"))
	} else {
		client.SetLogger(noopLogger{})
	}
	return client.
		SetRedirectPolicy(resty.NoRedirectPolicy()).
		SetHeader("Content-Type", jsonContainType).
		SetHeader("Accept", jsonContainType).
		SetJSONEscapeHTML(true).
		SetPreRequestHook(func(c *resty.Client, r *http.Request) error {
			schema := r.URL.Scheme
			if lo.Contains(schemaAllows, schema) {
				return nil
			}
			return errInvalidSchema
		}).
		SetRetryMaxWaitTime(config.RetryMaxWait).
		SetRetryWaitTime(config.RetryInterval).
		SetRetryCount(config.RetryCount)
}

type noopLogger struct {
}

func (n noopLogger) Errorf(format string, v ...interface{}) {
}

func (n noopLogger) Warnf(format string, v ...interface{}) {
}

func (n noopLogger) Debugf(format string, v ...interface{}) {
}
