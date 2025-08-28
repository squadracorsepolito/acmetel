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
	"github.com/squadracorsepolito/acmetel/raw"
)

const connectorSize = 4096

func main() {
	ctx, cancelCtx := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancelCtx()

	telemetry.Init(ctx, "kafka-example")

	tickerToRaw := connector.NewRingBuffer[*ingress.TickerMessage](connectorSize)
	rawToKafka := connector.NewRingBuffer[*egress.KafkaMessage](connectorSize)

	tickerCfg := ingress.DefaultTickerConfig()
	tickerCfg.Interval = time.Second
	tickerStage := ingress.NewTickerStage(tickerToRaw, tickerCfg)

	rawCfg := raw.NewDefaultConfig()
	rawStage := raw.NewStage("ticker_to_kafka", &rawHandler{}, tickerToRaw, rawToKafka, rawCfg)

	kafkaCfg := egress.DefaultKafkaConfig()
	kafkaStage := egress.NewKafkaStage(rawToKafka, kafkaCfg)

	pipeline := acmetel.NewPipeline()

	pipeline.AddStage(tickerStage)
	pipeline.AddStage(rawStage)
	pipeline.AddStage(kafkaStage)

	if err := pipeline.Init(ctx); err != nil {
		panic(err)
	}

	go pipeline.Run(ctx)
	defer pipeline.Close()

	<-ctx.Done()
}
