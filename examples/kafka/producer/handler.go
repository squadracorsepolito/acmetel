package main

import (
	"context"
	"strconv"

	"github.com/squadracorsepolito/acmetel/egress"
	"github.com/squadracorsepolito/acmetel/ingress"
)

type tickerToKafkaHandler struct{}

func newTickerToKafkaHandler() *tickerToKafkaHandler {
	return &tickerToKafkaHandler{}
}

func (h *tickerToKafkaHandler) Init(_ context.Context) error {
	return nil
}

func (h *tickerToKafkaHandler) Handle(_ context.Context, tickerMsg *ingress.TickerMessage, kafkaMsg *egress.KafkaMessage) error {
	tick := tickerMsg.TickNumber
	strTick := strconv.Itoa(tick)

	kafkaMsg.Topic = "example-topic"
	kafkaMsg.Key = []byte(strTick)
	kafkaMsg.Value = []byte(strTick)

	return nil
}

func (h *tickerToKafkaHandler) Close() {}
