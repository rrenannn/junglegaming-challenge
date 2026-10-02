package port

import "time"

// Metrics is the minimal set of business-level observations application
// services are allowed to emit. Concrete implementations live in
// infrastructure, which services may not import directly.
type Metrics interface {
	ObserveWagerOperation(kind, status string, duration time.Duration)
	ObserveWagerReplay()
	ObserveWagerConflict(reason string)
	ObserveReconciliationDivergence()
}

// NoopMetrics discards every observation. Used by tests that construct a
// service directly without wiring a real metrics backend.
type NoopMetrics struct{}

func (NoopMetrics) ObserveWagerOperation(string, string, time.Duration) {}
func (NoopMetrics) ObserveWagerReplay()                                 {}
func (NoopMetrics) ObserveWagerConflict(string)                         {}
func (NoopMetrics) ObserveReconciliationDivergence()                    {}
