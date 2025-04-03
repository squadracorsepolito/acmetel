package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/squadracorsepolito/acmetel"
	"github.com/squadracorsepolito/acmetel/core"
)

func main() {
	ctx, cancelCtx := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancelCtx()

	ingressToPreProc := acmetel.NewRingBuffer[[2048]byte](32000)
	preProcToProc := acmetel.NewRingBuffer[core.Message](32000)

	ingress := acmetel.NewUDPIngress(acmetel.NewDefaultUDPIngressConfig())
	ingress.SetOutput(ingressToPreProc)

	preProc := acmetel.NewCannelloniPreProcessor()
	preProc.SetInput(ingressToPreProc)
	preProc.SetOutput(preProcToProc)

	proc := acmetel.NewProcessor()
	proc.SetInput(preProcToProc)

	pipeline := acmetel.NewPipeline()

	pipeline.AddStage(ingress)
	pipeline.AddStage(acmetel.NewScaler(preProc, 2))
	pipeline.AddStage(acmetel.NewScaler(proc, 2))

	if err := pipeline.Init(ctx); err != nil {
		panic(err)
	}

	go pipeline.Run(ctx)
	defer pipeline.Stop()

	<-ctx.Done()
}
