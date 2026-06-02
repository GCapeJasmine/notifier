package kafka

import "github.com/prometheus/client_golang/prometheus"

var (
	kafkaOpsBuckets = prometheus.LinearBuckets(0.001, 0.005, 20)
	kafkaLagBuckets = prometheus.LinearBuckets(0.001, 0.01, 1000)

	writeMessageLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "Kafka__Write__Message__Latency",
		Help:    "Latency of write message in second",
		Buckets: kafkaOpsBuckets,
	})
	fetchMessageLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "Kafka__Fetch__Message__Latency",
		Help:    "Latency of fetch message in second",
		Buckets: kafkaOpsBuckets,
	})
	commitMessageLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "Kafka__Commit__Message__Latency",
		Help:    "Latency of commit message in second",
		Buckets: kafkaOpsBuckets,
	})
	fetchMessageLag = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "Kafka__Fetch__Message__Lag",
		Help:    "Lag (consume time - produce time) of fetch message in second",
		Buckets: kafkaLagBuckets,
	})
	commitMessageLag = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "Kafka__Commit__Message__Lag",
		Help:    "Lag (commit time - produce time) of commit message in second",
		Buckets: kafkaLagBuckets,
	})

	ApacheKafkaMetricsCollectors = []prometheus.Collector{
		writeMessageLatency,

		fetchMessageLatency,
		fetchMessageLag,

		commitMessageLatency,
		commitMessageLag,
	}
)
