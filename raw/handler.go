package raw

import (
	"context"

	"github.com/squadracorsepolito/acmetel/internal/message"
)

// Handler interface defines the methods that a custom handler for the raw stage must implement.
type Handler[In message.Message, T any, Out msgOut[T]] interface {
	// Init method is called once when the stage is initialized.
	Init(ctx context.Context) error

	// Handle method is called by one of the spawned workers
	// for each message received by the stage.
	Handle(ctx context.Context, msgIn In, msgOut Out) error

	// Close is called once when the stage is closed.
	Close()
}
