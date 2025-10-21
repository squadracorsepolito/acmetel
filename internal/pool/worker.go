package pool

import (
	"github.com/squadracorsepolito/acmetel/internal"
)

// BaseWorker is the base struct for a worker that can be embedded.
type BaseWorker struct {
	Tel *internal.Telemetry
}

// SetTelemetry sets the telemetry for the worker.
func (w *BaseWorker) SetTelemetry(tel *internal.Telemetry) {
	w.Tel = tel
}
