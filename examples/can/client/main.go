package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/squadracorsepolito/acmetel"
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/egress"
	"github.com/squadracorsepolito/acmetel/examples/telemetry"
	"github.com/squadracorsepolito/acmetel/ingress"
	"github.com/squadracorsepolito/acmetel/processor"
)

const connectorSize = 2048

func main() {
	ctx, cancelCtx := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancelCtx()

	telemetry.Init(ctx, "can-client-example")

	tickerToCustom := connector.NewRingBuffer[*ingress.TickerMessage](connectorSize)
	customToCannelloni := connector.NewRingBuffer[*processor.CannelloniMessage](connectorSize)
	cannelloniToUDP := connector.NewRingBuffer[*processor.CannelloniEncodedMessage](connectorSize)

	tickerCfg := ingress.DefaultTickerConfig()
	tickerCfg.Interval = time.Millisecond * 10
	tickerStage := ingress.NewTickerStage(tickerToCustom, tickerCfg)

	customCfg := processor.DefaultCustomConfig(acmetel.StageRunningModeSingle)
	customCfg.Name = "ticker_to_cannelloni"
	customStage := processor.NewCustomStage(newTickerToCannelloniHandler(), tickerToCustom, customToCannelloni, customCfg)

	cannelloniCfg := processor.DefaultCannelloniConfig(acmetel.StageRunningModeSingle)
	cannelloniStage := processor.NewCannelloniEncoderStage(customToCannelloni, cannelloniToUDP, cannelloniCfg)

	udpCfg := egress.DefaultUDPConfig(acmetel.StageRunningModeSingle)
	udpStage := egress.NewUDPStage(cannelloniToUDP, udpCfg)

	pipeline := acmetel.NewPipeline()

	pipeline.AddStage(tickerStage)
	pipeline.AddStage(customStage)
	pipeline.AddStage(cannelloniStage)
	pipeline.AddStage(udpStage)

	if err := pipeline.Init(ctx); err != nil {
		panic(err)
	}

	go pipeline.Run(ctx)
	defer pipeline.Close()

	<-ctx.Done()
}
