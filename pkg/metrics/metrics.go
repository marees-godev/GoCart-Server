package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gocart",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests processed partitioned by status code, method, and path.",
		},
		[]string{"service", "method", "path", "status_code"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gocart",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "Histogram of HTTP request latency in seconds.",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"service", "method", "path", "status_code"},
	)

	httpRequestsInFlight = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "gocart",
			Subsystem: "http",
			Name:      "requests_in_flight",
			Help:      "Current number of in-flight HTTP requests.",
		},
		[]string{"service"},
	)

	messageProcessedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gocart",
			Subsystem: "message",
			Name:      "processed_total",
			Help:      "Total number of asynchronous events/messages processed.",
		},
		[]string{"service", "topic", "status"},
	)

	messageProcessingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gocart",
			Subsystem: "message",
			Name:      "processing_duration_seconds",
			Help:      "Histogram of event processing latency in seconds.",
			Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"service", "topic", "status"},
	)

	messagePublishedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gocart",
			Subsystem: "message",
			Name:      "published_total",
			Help:      "Total number of asynchronous events published to message broker.",
		},
		[]string{"service", "topic", "status"},
	)

	dbQueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gocart",
			Subsystem: "database",
			Name:      "query_duration_seconds",
			Help:      "Histogram of database query latency in seconds.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
		},
		[]string{"service", "operation", "table"},
	)
)

func init() {
	prometheus.MustRegister(
		httpRequestsTotal,
		httpRequestDuration,
		httpRequestsInFlight,
		messageProcessedTotal,
		messageProcessingDuration,
		messagePublishedTotal,
		dbQueryDuration,
	)
}

func RecordHTTPRequest(service, method, path string, statusCode int, duration time.Duration) {
	statusStr := strconv.Itoa(statusCode)
	httpRequestsTotal.WithLabelValues(service, method, path, statusStr).Inc()
	httpRequestDuration.WithLabelValues(service, method, path, statusStr).Observe(duration.Seconds())
}

func IncHTTPInFlight(service string) {
	httpRequestsInFlight.WithLabelValues(service).Inc()
}

func DecHTTPInFlight(service string) {
	httpRequestsInFlight.WithLabelValues(service).Dec()
}

func RecordMessageProcessed(service, topic, status string, duration time.Duration) {
	messageProcessedTotal.WithLabelValues(service, topic, status).Inc()
	messageProcessingDuration.WithLabelValues(service, topic, status).Observe(duration.Seconds())
}

func RecordMessagePublished(service, topic, status string) {
	messagePublishedTotal.WithLabelValues(service, topic, status).Inc()
}

func RecordDBQuery(service, operation, table string, duration time.Duration) {
	dbQueryDuration.WithLabelValues(service, operation, table).Observe(duration.Seconds())
}

func Handler() http.Handler {
	return promhttp.Handler()
}
