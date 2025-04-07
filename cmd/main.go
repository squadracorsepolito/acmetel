package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/squadracorsepolito/acmetel"
	"github.com/squadracorsepolito/acmetel/adapter"
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/ingress"
	"github.com/squadracorsepolito/acmetel/processor"
)

func main() {
	ctx, cancelCtx := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancelCtx()

	ingressToAdapter := connector.NewRingBuffer[*ingress.UDPData](32000)
	adapterToProc := connector.NewRingBuffer[*adapter.CANMessageBatch](32000)

	ingressCfg := ingress.NewDefaultUDPConfig()
	ingressCfg.WorkerNum = 5
	ingress := ingress.NewUDP(ingressCfg)
	ingress.SetOutput(ingressToAdapter)

	adapter := adapter.NewCannelloni(&adapter.CannelloniConfig{WorkerNum: 8})
	adapter.SetInput(ingressToAdapter)
	adapter.SetOutput(adapterToProc)

	proc := processor.NewProcessor()
	proc.SetInput(adapterToProc)

	pipeline := acmetel.NewPipeline()

	pipeline.AddStage(ingress)
	pipeline.AddStage(adapter)
	pipeline.AddStage(proc)

	if err := pipeline.Init(ctx); err != nil {
		panic(err)
	}

	go pipeline.Run(ctx)
	defer pipeline.Stop()

	<-ctx.Done()
}
