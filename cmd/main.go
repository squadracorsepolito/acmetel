package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel"
	"github.com/squadracorsepolito/acmetel/adapter"
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/ingress"
	"github.com/squadracorsepolito/acmetel/processor"
)

func main() {
	ctx, cancelCtx := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancelCtx()

	ingressToAdapter := connector.NewRingBuffer[*ingress.UDPData](16_000)
	adapterToProc := connector.NewRingBuffer[*adapter.CANMessageBatch](16_000)

	ingressCfg := ingress.NewDefaultUDPConfig()
	ingress := ingress.NewUDP(ingressCfg)
	ingress.SetOutput(ingressToAdapter)

	cannelloniCfg := adapter.NewDefaultCannelloniConfig()
	adapter := adapter.NewCannelloni(cannelloniCfg)
	adapter.SetInput(ingressToAdapter)
	adapter.SetOutput(adapterToProc)

	acmelibCfg := processor.NewDefaultAcmelibConfig()
	acmelibCfg.Messages = getMessages()
	proc := processor.NewAcmelib(acmelibCfg)
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

func getMessages() []*acmelib.Message {
	messages := []*acmelib.Message{}

	sigType, _ := acmelib.NewIntegerSignalType("sig_type", 8, false)
	for i := range 113 {
		msg := acmelib.NewMessage(fmt.Sprintf("message_%d", i), acmelib.MessageID(i), 8)

		for j := range 8 {
			sig, _ := acmelib.NewStandardSignal(fmt.Sprintf("message_%d_signal_%d", i, j), sigType)

			if err := msg.InsertSignal(sig, j*8); err != nil {
				panic(err)
			}
		}

		messages = append(messages, msg)
	}

	return messages
}
