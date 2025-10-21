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

	telemetry.Init(ctx, "kafka-example")

	tickerToCustom := connector.NewRingBuffer[*ingress.TickerMessage](connectorSize)
	customToKafka := connector.NewRingBuffer[*egress.KafkaMessage](connectorSize)

	tickerCfg := ingress.DefaultTickerConfig()
	tickerCfg.Interval = time.Second
	tickerStage := ingress.NewTickerStage(tickerToCustom, tickerCfg)

	customCfg := processor.DefaultCustomConfig(acmetel.StageRunningModePool)
	customCfg.Name = "ticker_to_kafka"
	customStage := processor.NewCustomStage(newTickerToKafkaHandler(), tickerToCustom, customToKafka, customCfg)

	kafkaCfg := egress.DefaultKafkaConfig(acmetel.StageRunningModePool)
	kafkaStage := egress.NewKafkaStage(customToKafka, kafkaCfg)

	pipeline := acmetel.NewPipeline()

	pipeline.AddStage(tickerStage)
	pipeline.AddStage(customStage)
	pipeline.AddStage(kafkaStage)

	if err := pipeline.Init(ctx); err != nil {
		panic(err)
	}

	go pipeline.Run(ctx)
	defer pipeline.Close()

	<-ctx.Done()
}
