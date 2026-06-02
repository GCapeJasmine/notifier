package telemetry

import (
	"fmt"
	"net/http"

	"github.com/gleo/subscribers/common/log"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	DefaultMetricPort = 6067
)

type ExporterConfig struct {
	Enable bool `yaml:"enable" mapstructure:"enable"`
	Port   int  `yaml:"port" mapstructure:"port"`
}

type Exporter struct {
	config ExporterConfig
}

func NewExporter(config ExporterConfig) *Exporter {
	return &Exporter{config: config}
}

func (p *Exporter) Start() error {
	if p.config.Enable {
		port := DefaultMetricPort
		if p.config.Port > 0 {
			port = p.config.Port
		}
		log.Logger.Infof("starting metric exporter at port %d", port)
		return http.ListenAndServe(fmt.Sprintf(":%d", port), promhttp.Handler())
	}

	return nil
}
