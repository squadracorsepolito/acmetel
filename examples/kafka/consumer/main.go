package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

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

	kafkaToRaw := connector.NewRingBuffer[*ingress.KafkaMessage](connectorSize)
	rawToKafka := connector.NewRingBuffer[*egress.KafkaMessage](connectorSize)

	kafkaIngressCfg := ingress.DefaultKafkaConfig("example-topic")
	kafkaIngressStage := ingress.NewKafkaStage(kafkaToRaw, kafkaIngressCfg)

	rawCfg := raw.NewDefaultConfig()
	rawStage := raw.NewStage("ingress_to_egress", &rawHandler{}, kafkaToRaw, rawToKafka, rawCfg)

	kafkaEgressCfg := egress.DefaultKafkaConfig()
	kafkaEgressStage := egress.NewKafkaStage(rawToKafka, kafkaEgressCfg)

	pipeline := acmetel.NewPipeline()

	pipeline.AddStage(kafkaIngressStage)
	pipeline.AddStage(rawStage)
	pipeline.AddStage(kafkaEgressStage)

	if err := pipeline.Init(ctx); err != nil {
		panic(err)
	}

	go pipeline.Run(ctx)
	defer pipeline.Close()

	<-ctx.Done()
}
