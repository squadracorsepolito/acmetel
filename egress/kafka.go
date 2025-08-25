package egress

import (
	"context"
	"time"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal/message"
	"github.com/squadracorsepolito/acmetel/internal/pool"
	"github.com/squadracorsepolito/acmetel/internal/stage"
	"github.com/squadracorsepolito/acmetel/internal/telemetry"

	"github.com/segmentio/kafka-go"
)

//////////////
//  CONFIG  //
//////////////

type KafkaConfig struct {
	PoolConfig *pool.Config

	Brokers []string

	// The balancer used to distribute messages across partitions.
	//
	// The default is to use a round-robin distribution.
	Balancer kafka.Balancer

	// Limit on how many attempts will be made to deliver a message.
	//
	// The default is to try at most 10 times.
	MaxAttempts int

	// WriteBackoffMin optionally sets the smallest amount of time the writer waits before
	// it attempts to write a batch of messages
	//
	// Default: 100ms
	WriteBackoffMin time.Duration

	// WriteBackoffMax optionally sets the maximum amount of time the writer waits before
	// it attempts to write a batch of messages
	//
	// Default: 1s
	WriteBackoffMax time.Duration

	// Limit on how many messages will be buffered before being sent to a
	// partition.
	//
	// The default is to use a target batch size of 100 messages.
	BatchSize int

	// Limit the maximum size of a request in bytes before being sent to
	// a partition.
	//
	// The default is to use a kafka default value of 1048576.
	BatchBytes int64

	// Time limit on how often incomplete message batches will be flushed to
	// kafka.
	//
	// The default is to flush at least every second.
	BatchTimeout time.Duration

	// Timeout for read operations performed by the Writer.
	//
	// Defaults to 10 seconds.
	ReadTimeout time.Duration

	// Timeout for write operation performed by the Writer.
	//
	// Defaults to 10 seconds.
	WriteTimeout time.Duration

	// Number of acknowledges from partition replicas required before receiving
	// a response to a produce request, the following values are supported:
	//
	//  RequireNone (0)  fire-and-forget, do not wait for acknowledgements from the
	//  RequireOne  (1)  wait for the leader to acknowledge the writes
	//  RequireAll  (-1) wait for the full ISR to acknowledge the writes
	//
	// Defaults to RequireNone.
	RequiredAcks kafka.RequiredAcks

	// Setting this flag to true causes the WriteMessages method to never block.
	// It also means that errors are ignored since the caller will not receive
	// the returned value. Use this only if you don't care about guarantees of
	// whether the messages were written to kafka.
	//
	// Defaults to true.
	Async bool

	// Compression set the compression codec to be used to compress messages.
	Compression kafka.Compression

	// A transport used to send messages to kafka clusters.
	//
	// If nil, DefaultTransport is used.
	Transport kafka.RoundTripper

	// AllowAutoTopicCreation notifies writer to create topic if missing.
	AllowAutoTopicCreation bool
}

func DefaultKafkaConfig() *KafkaConfig {
	return &KafkaConfig{
		PoolConfig: pool.DefaultConfig(),

		Brokers:                []string{"localhost:9092"},
		Balancer:               &kafka.RoundRobin{},
		MaxAttempts:            10,
		WriteBackoffMin:        100 * time.Millisecond,
		WriteBackoffMax:        1 * time.Second,
		BatchSize:              100,
		BatchBytes:             1048576,
		BatchTimeout:           time.Second,
		ReadTimeout:            10 * time.Second,
		WriteTimeout:           10 * time.Second,
		RequiredAcks:           kafka.RequireNone,
		Async:                  true,
		Compression:            kafka.Snappy,
		AllowAutoTopicCreation: true,
	}
}

///////////////
//  MESSAGE  //
///////////////

type KafkaMessage struct {
	message.Base

	Topic string
	Key   []byte
	Value []byte

	headers []kafka.Header
}

func (km *KafkaMessage) AddHeader(key string, value []byte) {
	km.headers = append(km.headers, kafka.Header{
		Key:   key,
		Value: value,
	})
}

//////////////
//  WORKER  //
//////////////

type kafkaWorkerArgs struct {
	writer *kafka.Writer
}

func newKafkaWorkerArgs(writer *kafka.Writer) *kafkaWorkerArgs {
	return &kafkaWorkerArgs{
		writer: writer,
	}
}

type kafkaWorker struct {
	pool.BaseWorker

	writer *kafka.Writer
}

func (kw *kafkaWorker) Init(_ context.Context, args *kafkaWorkerArgs) error {
	kw.writer = args.writer

	return nil
}

func (kw *kafkaWorker) Deliver(ctx context.Context, msg *KafkaMessage) error {
	// Extract the span context from the input message
	ctx, span := kw.Tel.NewTrace(msg.LoadSpanContext(ctx), "deliver kafka message")
	defer span.End()

	// Create the header that carries the trace and eventual user defined headers
	headerCarrier := telemetry.NewKafkaHeaderCarrier(msg.headers)

	// Inject the trace
	kw.Tel.InjectTrace(ctx, headerCarrier)

	// Create the message to be written
	kafkaMsg := kafka.Message{
		Topic: msg.Topic,
		Key:   msg.Key,
		Value: msg.Value,

		Headers: headerCarrier.Headers(),
	}

	// Write the message to kafka
	if err := kw.writer.WriteMessages(ctx, kafkaMsg); err != nil {
		return err
	}

	return nil
}

func (kw *kafkaWorker) Close(_ context.Context) error { return nil }

/////////////
//  STAGE  //
/////////////

type KafkaStage struct {
	*stage.Egress[*KafkaMessage, kafkaWorker, *kafkaWorkerArgs, *kafkaWorker]

	cfg *KafkaConfig

	writer *kafka.Writer
}

func NewKafkaStage(inputConnector connector.Connector[*KafkaMessage], cfg *KafkaConfig) *KafkaStage {
	return &KafkaStage{
		Egress: stage.NewEgress[*KafkaMessage, kafkaWorker, *kafkaWorkerArgs]("kafka", inputConnector, cfg.PoolConfig),

		cfg: cfg,
	}
}

func (ks *KafkaStage) Init(ctx context.Context) error {
	ks.writer = &kafka.Writer{
		Addr:                   kafka.TCP(ks.cfg.Brokers...),
		Balancer:               ks.cfg.Balancer,
		MaxAttempts:            ks.cfg.MaxAttempts,
		WriteBackoffMin:        ks.cfg.WriteBackoffMin,
		WriteBackoffMax:        ks.cfg.WriteBackoffMax,
		BatchSize:              ks.cfg.BatchSize,
		BatchBytes:             ks.cfg.BatchBytes,
		BatchTimeout:           ks.cfg.BatchTimeout,
		ReadTimeout:            ks.cfg.ReadTimeout,
		WriteTimeout:           ks.cfg.WriteTimeout,
		RequiredAcks:           ks.cfg.RequiredAcks,
		Async:                  ks.cfg.Async,
		Compression:            ks.cfg.Compression,
		Transport:              ks.cfg.Transport,
		AllowAutoTopicCreation: ks.cfg.AllowAutoTopicCreation,
	}

	return ks.Egress.Init(ctx, newKafkaWorkerArgs(ks.writer))
}

func (ks *KafkaStage) Close() {
	ks.Egress.Close()

	if err := ks.writer.Close(); err != nil {
		ks.Tel.LogError("failed to close writer", err)
	}
}
