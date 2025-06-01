package egress

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/message"
	"github.com/squadracorsepolito/acmetel/worker"
)

type RedisTimeSeriesConfig struct {
	*worker.PoolConfig

	Addr string
}

func DefaultRedisTimeSeriesConfig() *RedisTimeSeriesConfig {
	return &RedisTimeSeriesConfig{
		PoolConfig: worker.DefaultPoolConfig(),

		Addr: "localhost:6379",
	}
}

type RedisTimeSeries struct {
	*stage[*message.CANSignalBatch, *RedisTimeSeriesConfig, redisTimeSeriesWorker, *redis.Client, *redisTimeSeriesWorker]
}

func NewRedisTimeSeries(cfg *RedisTimeSeriesConfig) *RedisTimeSeries {
	return &RedisTimeSeries{
		stage: newStage[*message.CANSignalBatch, *RedisTimeSeriesConfig, redisTimeSeriesWorker, *redis.Client]("redis_ts", cfg),
	}
}

func (e *RedisTimeSeries) Init(ctx context.Context) error {
	client := redis.NewClient(&redis.Options{
		Addr:     e.cfg.Addr,
		DB:       0,
		Password: "",
	})

	// Create the tables
	// keyname := "int_signals"
	// _, err := client.Info(keyname)
	// if err != nil {
	// 	opts := redists.DefaultCreateOptions
	// 	opts.DuplicatePolicy = "LAST"
	// 	if err := client.CreateKeyWithOptions(keyname, opts); err != nil {
	// 		return err
	// 	}
	// }

	return e.init(ctx, client)
}

func (e *RedisTimeSeries) Run(ctx context.Context) {
	e.run(ctx)
}

func (e *RedisTimeSeries) Stop() {
	e.close()
}

type redisTimeSeriesWorker struct {
	tel *internal.Telemetry

	client *redis.Client
	pipe   redis.Pipeliner

	count int
}

func (w *redisTimeSeriesWorker) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
}

func (w *redisTimeSeriesWorker) Init(ctx context.Context, client *redis.Client) error {
	w.client = client
	return nil
}

func (w *redisTimeSeriesWorker) Deliver(ctx context.Context, data *message.CANSignalBatch) error {
	defer message.PutCANSignalBatch(data)

	if w.count == 0 {
		w.pipe = w.client.Pipeline()
	}

	w.count += data.SignalCount

	timestamp := data.Timestamp.UnixMilli()

	for i := range data.SignalCount {
		sig := data.Signals[i]

		key := sig.Name
		value := float64(0)

		switch sig.Table {
		case message.CANSignalTableInt:
			value = float64(sig.ValueInt)
		}

		w.pipe.TSAddWithArgs(ctx, key, timestamp, value, &redis.TSOptions{
			DuplicatePolicy: "LAST",
		})
	}

	if w.count > 10_000 {
		w.count = 0

		_, err := w.pipe.Exec(ctx)
		if err != nil {
			return err
		}

		w.pipe = w.client.Pipeline()
	}

	return nil
}

func (w *redisTimeSeriesWorker) Stop(_ context.Context) error {
	return nil
}
