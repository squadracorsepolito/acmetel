package stage

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
)

type Source[Out msg] interface {
	SetTelemetry(*internal.Telemetry)
	Run(context.Context, chan<- Out)
}

type Ingress[Out msg] struct {
	tel *internal.Telemetry

	source Source[Out]

	outputConnector connector.Connector[Out]

	writerInputCh chan Out
	writerWg      *sync.WaitGroup

	// Telemetry metrics
	receivedMessages atomic.Int64
}

func NewIngress[Out msg](name string, source Source[Out], outputConnector connector.Connector[Out], writerQueueSize int) *Ingress[Out] {
	tel := internal.NewTelemetry("ingress", name)

	source.SetTelemetry(tel)

	return &Ingress[Out]{
		tel: tel,

		source: source,

		outputConnector: outputConnector,

		writerInputCh: make(chan Out, writerQueueSize),
		writerWg:      &sync.WaitGroup{},
	}
}

func (i *Ingress[Out]) Init(_ context.Context) error {
	i.initMetrics()

	return nil
}

func (i *Ingress[Out]) initMetrics() {
	i.tel.NewCounter("received_messages", func() int64 { return i.receivedMessages.Load() })
}

func (i *Ingress[M]) Run(ctx context.Context) {
	go i.runWriter(ctx)

	i.source.Run(ctx, i.writerInputCh)
}

func (i *Ingress[M]) runWriter(ctx context.Context) {
	i.writerWg.Add(1)
	defer i.writerWg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case msgOut := <-i.writerInputCh:
			i.receivedMessages.Add(1)

			if err := i.outputConnector.Write(msgOut); err != nil {
				i.tel.LogError("failed to write into output connector", err)
			}
		}
	}
}

func (i *Ingress[M]) Close() {
	i.tel.LogInfo("closing")

	i.outputConnector.Close()
	i.writerWg.Wait()

	close(i.writerInputCh)
}
