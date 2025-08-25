package main

import (
	"context"

	"github.com/squadracorsepolito/acmetel/egress"
	"github.com/squadracorsepolito/acmetel/ingress"
)

type rawHandler struct{}

func (h *rawHandler) Init(_ context.Context) error {
	return nil
}

func (h *rawHandler) Handle(_ context.Context, kafkaIngressMsg *ingress.KafkaMessage, kafkaEgressMsg *egress.KafkaMessage) error {
	// log.Print(string(kafkaIngressMsg.Value))

	kafkaEgressMsg.Topic = "return-topic"
	kafkaEgressMsg.Key = kafkaIngressMsg.Key
	kafkaEgressMsg.Value = kafkaIngressMsg.Value

	return nil
}

func (h *rawHandler) Close() {}
