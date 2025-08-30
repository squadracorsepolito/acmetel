package stage

import (
	"context"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
)

type Source[Out msg] interface {
	SetTelemetry(*internal.Telemetry)
	Run(context.Context, connector.Connector[Out])
}

type Ingress[Out msg] struct {
	tel *internal.Telemetry

	source Source[Out]

	outputConnector connector.Connector[Out]
}

func NewIngress[Out msg](name string, source Source[Out], outputConnector connector.Connector[Out] /*writerQueueSize int*/) *Ingress[Out] {
	tel := internal.NewTelemetry("ingress", name)

	source.SetTelemetry(tel)

	return &Ingress[Out]{
		tel: tel,

		source: source,

		outputConnector: outputConnector,
	}
}

func (i *Ingress[Out]) Init(_ context.Context) error {
	i.tel.LogInfo("initializing")
	defer i.tel.LogInfo("initialized")

	return nil
}

func (i *Ingress[M]) Run(ctx context.Context) {
	i.source.Run(ctx, i.outputConnector)
}

func (i *Ingress[M]) Close() {
	i.tel.LogInfo("closing")
	defer i.tel.LogInfo("closed")

	i.outputConnector.Close()
}
