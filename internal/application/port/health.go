package port

import "context"

// HealthCheck represents a dependency required for the application to be ready.
type HealthCheck interface {
	Name() string
	Check(context.Context) error
}
