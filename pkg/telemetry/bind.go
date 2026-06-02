package telemetry

import "github.com/google/wire"

var (
	ExporterProviderSet = wire.NewSet(
		NewExporter,
		GetMeterProvider,
	)
)
