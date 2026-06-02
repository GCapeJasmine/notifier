package telemetry

import (
	"sync"
	"time"

	"github.com/gleo/subscribers/pkg/utils"
	"go.opentelemetry.io/otel/exporters/prometheus"
	metric2 "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
)

const (
	OtelDefaultTimeout = time.Second
)

var (
	meterProvider     metric2.MeterProvider
	meterProviderOnce sync.Once
)

func GetMeterProvider() metric2.MeterProvider {
	meterProviderOnce.Do(func() {
		exporter, err := prometheus.New()
		utils.PanicOnError(err)
		meterProvider = metric.NewMeterProvider(metric.WithReader(exporter))
	})

	return meterProvider
}
