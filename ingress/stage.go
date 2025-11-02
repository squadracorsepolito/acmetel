package ingress

import (
	"context"

	"github.com/squadracorsepolito/acmetel/internal"
)

type source[Out msgEnv] interface {
	SetTelemetry(tel *internal.Telemetry)
	Run(ctx context.Context, outputConnector msgConn[Out])
}

type stage[Out msgEnv] struct {
	tel *internal.Telemetry

	source source[Out]

	outputConnector msgConn[Out]
}

func newStage[Out msgEnv](name string, source source[Out], outConn msgConn[Out]) *stage[Out] {
	tel := internal.NewTelemetry("ingress", name)
	source.SetTelemetry(tel)

	return &stage[Out]{
		tel: tel,

		source: source,

		outputConnector: outConn,
	}
}

func (s *stage[Out]) Init(_ context.Context) error {
	s.tel.LogInfo("initializing")

	return nil
}

func (s *stage[Out]) Run(ctx context.Context) {
	s.source.Run(ctx, s.outputConnector)
}

func (s *stage[Out]) Close() {
	s.tel.LogInfo("closing")

	// Close the output connector
	s.outputConnector.Close()
}
