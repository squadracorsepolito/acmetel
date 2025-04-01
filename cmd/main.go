package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/squadracorsepolito/acmetel"
	"github.com/squadracorsepolito/acmetel/core"
	"github.com/squadracorsepolito/acmetel/internal"
)

func main() {
	ctx, cancelCtx := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancelCtx()

	ingressToPreProc := internal.NewRingBuffer[[]byte](1024)
	preProcToProc := internal.NewRingBuffer[*core.Message](1024)

	ingress := acmetel.NewUDPIngress(acmetel.NewDefaultUDPIngressConfig())
	ingress.SetOutput(ingressToPreProc)

	preProc := acmetel.NewCannelloniPreProcessor()
	preProc.SetInput(ingressToPreProc)
	preProc.SetOutput(preProcToProc)

	proc := acmetel.NewProcessor()
	proc.SetInput(preProcToProc)

	pipeline := acmetel.NewPipeline()

	pipeline.AddStage(ingress)
	pipeline.AddStage(preProc)
	pipeline.AddStage(proc)

	if err := pipeline.Init(ctx); err != nil {
		panic(err)
	}

	go pipeline.Run(ctx)
	defer pipeline.Stop()

	<-ctx.Done()
}
