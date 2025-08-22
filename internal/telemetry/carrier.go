package telemetry

import (
	"slices"

	"github.com/segmentio/kafka-go"
)

const (
	textMapPropagatorFields = 1
)

type KafkaHeaderCarrier struct {
	headers []kafka.Header
}

func NewKafkaHeaderCarrier(additionalHeaders int) *KafkaHeaderCarrier {
	return &KafkaHeaderCarrier{
		headers: make([]kafka.Header, 0, additionalHeaders+textMapPropagatorFields),
	}
}

func (khc *KafkaHeaderCarrier) Get(key string) string {
	for _, header := range khc.headers {
		if key == header.Key {
			return string(header.Value)
		}
	}
	return ""
}

func (khc *KafkaHeaderCarrier) Set(key, value string) {
	khc.headers = slices.DeleteFunc(khc.headers, func(header kafka.Header) bool {
		return header.Key == key
	})

	khc.headers = append(khc.headers, kafka.Header{
		Key:   key,
		Value: []byte(value),
	})
}

func (khc *KafkaHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(khc.headers))
	for _, header := range khc.headers {
		keys = append(keys, header.Key)
	}
	return keys
}

func (khc *KafkaHeaderCarrier) AppendHeaders(headers ...kafka.Header) {
	khc.headers = append(khc.headers, headers...)
}

func (khc *KafkaHeaderCarrier) Headers() []kafka.Header {
	return khc.headers
}
