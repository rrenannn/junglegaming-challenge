package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics implements port.Metrics for application services, and exposes a
// few extra methods used directly by adapters (SQS consumer, outbox
// publisher), which are allowed to depend on infrastructure.
type Metrics struct {
	wagerOperations           *prometheus.CounterVec
	wagerDuration             *prometheus.HistogramVec
	wagerReplays              prometheus.Counter
	wagerConflicts            *prometheus.CounterVec
	reconciliationDivergences prometheus.Counter
	outboxPending             prometheus.Gauge
	outboxPendingOldestAge    prometheus.Gauge
	sqsMessages               *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	return &Metrics{
		wagerOperations: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "wager_operations_total",
			Help: "Total wager operations processed, by kind and resulting status.",
		}, []string{"kind", "status"}),
		wagerDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name: "wager_operation_duration_seconds",
			Help: "Time to process a wager operation end to end, by kind.",
		}, []string{"kind"}),
		wagerReplays: promauto.NewCounter(prometheus.CounterOpts{
			Name: "wager_replays_total",
			Help: "Total resubmissions recognized as a safe replay of an already-processed operation.",
		}),
		wagerConflicts: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "wager_conflicts_total",
			Help: "Total rejected resubmissions whose key matched a prior operation with different content.",
		}, []string{"reason"}),
		reconciliationDivergences: promauto.NewCounter(prometheus.CounterOpts{
			Name: "reconciliation_divergences_total",
			Help: "Total times a wallet reconciliation found the recorded balance did not match the ledger.",
		}),
		outboxPending: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_pending_events",
			Help: "Current number of outbox events not yet published.",
		}),
		outboxPendingOldestAge: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_pending_oldest_age_seconds",
			Help: "Age of the oldest unpublished outbox event, in seconds.",
		}),
		sqsMessages: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "sqs_messages_processed_total",
			Help: "Total SQS wager messages handled, by outcome.",
		}, []string{"result"}),
	}
}

func (m *Metrics) ObserveWagerOperation(kind, status string, duration time.Duration) {
	m.wagerOperations.WithLabelValues(kind, status).Inc()
	m.wagerDuration.WithLabelValues(kind).Observe(duration.Seconds())
}

func (m *Metrics) ObserveWagerReplay() {
	m.wagerReplays.Inc()
}

func (m *Metrics) ObserveWagerConflict(reason string) {
	m.wagerConflicts.WithLabelValues(reason).Inc()
}

func (m *Metrics) ObserveReconciliationDivergence() {
	m.reconciliationDivergences.Inc()
}

func (m *Metrics) ObserveSQSMessage(result string) {
	m.sqsMessages.WithLabelValues(result).Inc()
}

func (m *Metrics) SetOutboxPending(count int, oldestAgeSeconds float64) {
	m.outboxPending.Set(float64(count))
	m.outboxPendingOldestAge.Set(oldestAgeSeconds)
}
