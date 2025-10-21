package ingress

import (
	"context"

	"github.com/squadracorsepolito/acmetel/internal"
)

type source[Out msg] interface {
	SetTelemetry(tel *internal.Telemetry)
	Run(ctx context.Context, outputConnector conn[Out])
}

type stage[Out msg] struct {
	tel *internal.Telemetry

	source source[Out]

	outputConnector conn[Out]
}

func newStage[Out msg](name string, source source[Out], outConn conn[Out]) *stage[Out] {
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
