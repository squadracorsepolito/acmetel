package stage

import (
	"context"
	"errors"
	"sync"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/pool"
	"github.com/squadracorsepolito/acmetel/internal/rb"
)

type Processor[MIn, MOut msg, W, WArgs any, WPtr processorWPtr[W, WArgs, MIn, MOut]] struct {
	tel *internal.Telemetry

	inputConnector  connector.Connector[MIn]
	outputConnector connector.Connector[MOut]

	writerWg *sync.WaitGroup

	workerPool *pool.Processor[W, WArgs, MIn, MOut, WPtr]
}

func NewProcessor[MIn, MOut msg, W, WArgs any, WPtr processorWPtr[W, WArgs, MIn, MOut]](
	name string, inputConnector connector.Connector[MIn], outputConnector connector.Connector[MOut], poolCfg *pool.Config,
) *Processor[MIn, MOut, W, WArgs, WPtr] {

	tel := internal.NewTelemetry("processor", name)

	return &Processor[MIn, MOut, W, WArgs, WPtr]{
		tel: tel,

		inputConnector:  inputConnector,
		outputConnector: outputConnector,

		writerWg: &sync.WaitGroup{},

		workerPool: pool.NewProcessor[W, WArgs, MIn, MOut, WPtr](tel, poolCfg),
	}
}

func (p *Processor[MIn, MOut, W, WArgs, WPtr]) Init(ctx context.Context, workerArgs WArgs) error {
	defer p.tel.LogInfo("initialized")

	p.workerPool.Init(ctx, workerArgs)

	return nil
}

func (p *Processor[MIn, MOut, W, WArgs, WPtr]) runWriter(ctx context.Context) {
	p.writerWg.Add(1)
	defer p.writerWg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgOut, err := p.workerPool.ExtractMessage()
		if err != nil {
			continue
		}

		if err := p.outputConnector.Write(msgOut); err != nil {
			p.tel.LogError("failed to write into output connector", err)
		}
	}
}

func (p *Processor[MIn, MOut, W, WArgs, WPtr]) Run(ctx context.Context) {
	p.tel.LogInfo("running")
	defer p.tel.LogInfo("stopped")

	// Run the worker pool
	go p.workerPool.Run(ctx)

	// Run the writer goroutine
	go p.runWriter(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		default:
		}

		msg, err := p.inputConnector.Read()
		if err != nil {
			// Check if the input connector is closed, if so stop
			if errors.Is(err, connector.ErrClosed) {
				p.tel.LogInfo("input connector is closed, stopping")
				return
			}

			if !errors.Is(err, rb.ErrReadTimeout) {
				p.tel.LogError("failed to read from input connector", err)
			}

			continue
		}

		// Push a new task to the worker pool
		if err := p.workerPool.AddMessage(ctx, msg); err != nil {
			p.tel.LogError("failed to add message to worker pool", err)
			continue
		}
	}
}

func (p *Processor[MIn, MOut, W, WArgs, WPtr]) Close() {
	p.tel.LogInfo("closing")

	// Close the output connector
	p.outputConnector.Close()
	p.workerPool.Close()

	// Wait for the writer to finish
	p.writerWg.Wait()
}
