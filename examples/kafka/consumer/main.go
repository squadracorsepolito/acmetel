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
	"github.com/squadracorsepolito/acmetel/processor"
)

const connectorSize = 2048

func main() {
	ctx, cancelCtx := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancelCtx()

	telemetry.Init(ctx, "kafka-example")

	kafkaToRaw := connector.NewRingBuffer[*ingress.KafkaMessage](connectorSize)
	customToKafka := connector.NewRingBuffer[*egress.KafkaMessage](connectorSize)

	kafkaIngressCfg := ingress.DefaultKafkaConfig("example-topic")
	kafkaIngressStage := ingress.NewKafkaStage(kafkaToRaw, kafkaIngressCfg)

	customCfg := processor.DefaultCustomConfig()
	customCfg.Name = "ingress_to_egress"
	customStage := processor.NewCustomStage(newIngressToEgressHandler(), kafkaToRaw, customToKafka, customCfg)

	kafkaEgressCfg := egress.DefaultKafkaConfig()
	kafkaEgressStage := egress.NewKafkaStage(customToKafka, kafkaEgressCfg)

	pipeline := acmetel.NewPipeline()

	pipeline.AddStage(kafkaIngressStage)
	pipeline.AddStage(customStage)
	pipeline.AddStage(kafkaEgressStage)

	if err := pipeline.Init(ctx); err != nil {
		panic(err)
	}

	go pipeline.Run(ctx)
	defer pipeline.Close()

	<-ctx.Done()
}
