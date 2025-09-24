package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel"
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/egress"
	"github.com/squadracorsepolito/acmetel/ingress"
	"github.com/squadracorsepolito/acmetel/processor"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

const connectorSize = 2048

func main() {
	ctx, cancelCtx := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancelCtx()

	// Telemetry
	resource := newResource()
	// Trace
	traceExporter := newTraceExporter(ctx)
	traceProvider := newTraceProvider(resource, traceExporter)
	defer traceProvider.Shutdown(context.Background())
	otel.SetTracerProvider(traceProvider)
	// Meter
	meterExporter := newMeterExporter(ctx)
	meterProvider := newMeterProvider(resource, meterExporter)
	defer meterProvider.Shutdown(ctx)
	otel.SetMeterProvider(meterProvider)
	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		panic(err)
	}

	udpToCannelloni := connector.NewRingBuffer[*ingress.UDPMessage](connectorSize)
	cannelloniToROB := connector.NewRingBuffer[*processor.CannelloniMessage](connectorSize)
	robToCAN := connector.NewRingBuffer[*processor.CannelloniMessage](connectorSize)
	canToCustom := connector.NewRingBuffer[*processor.CANMessage](connectorSize)
	customToQuestDB := connector.NewRingBuffer[*egress.QuestDBMessage](connectorSize)

	udpCfg := ingress.DefaultUDPConfig()
	udpStage := ingress.NewUDPStage(udpToCannelloni, udpCfg)

	cannelloniCfg := processor.DefaultCannelloniConfig()
	cannelloniStage := processor.NewCannelloniDecoderStage(udpToCannelloni, cannelloniToROB, cannelloniCfg)

	robCfg := processor.DefaultROBConfig()
	robStage := processor.NewROBStage(cannelloniToROB, robToCAN, robCfg)

	canCfg := processor.DefaultCANConfig()
	canCfg.Messages = getMessages()
	canStage := processor.NewCANStage(robToCAN, canToCustom, canCfg)

	customCfg := processor.DefaultCustomConfig()
	customCfg.Name = "can_to_questdb"
	customCfg.PoolConfig.MinWorkers = customCfg.PoolConfig.InitialWorkers
	customStage := processor.NewCustomStage(newCANToQuestDBHandler(), canToCustom, customToQuestDB, customCfg)

	questDBCfg := egress.DefaultQuestDBConfig()
	questDBCfg.PoolConfig.MinWorkers = questDBCfg.PoolConfig.InitialWorkers
	questDBStage := egress.NewQuestDBStage(customToQuestDB, questDBCfg)

	pipeline := acmetel.NewPipeline()

	pipeline.AddStage(udpStage)
	pipeline.AddStage(cannelloniStage)
	pipeline.AddStage(robStage)
	pipeline.AddStage(canStage)
	pipeline.AddStage(customStage)
	pipeline.AddStage(questDBStage)

	if err := pipeline.Init(ctx); err != nil {
		panic(err)
	}

	go pipeline.Run(ctx)
	defer pipeline.Close()

	<-ctx.Done()
}

func getMessages() []*acmelib.Message {
	messages := []*acmelib.Message{}

	sigType, _ := acmelib.NewIntegerSignalType("sig_type", 8, false)
	msg := acmelib.NewMessage("message_0", acmelib.MessageID(1), 8)

	for j := range 8 {
		sig, _ := acmelib.NewStandardSignal(fmt.Sprintf("message_0_signal_%d", j), sigType)

		if err := msg.InsertSignal(sig, j*8); err != nil {
			panic(err)
		}
	}

	messages = append(messages, msg)

	// dbcFile, err := os.Open("MCB.dbc")
	// if err != nil {
	// 	panic(err)
	// }
	// defer dbcFile.Close()
	// bus, err := acmelib.ImportDBCFile("MCB", dbcFile)
	// if err != nil {
	// 	panic(err)
	// }

	// for _, nodeInt := range bus.NodeInterfaces() {
	// 	for _, msg := range nodeInt.SentMessages() {
	// 		messages = append(messages, msg)
	// 	}
	// }

	return messages
}

func newResource() *resource.Resource {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("sc-test-telemetry"),
			semconv.ServiceVersion("0.1.0"),
		),
	)

	if err != nil {
		panic(err)
	}

	return res
}

func newTraceExporter(ctx context.Context) *otlptrace.Exporter {
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	return exporter
}

func newTraceProvider(resource *resource.Resource, exporter sdktrace.SpanExporter) *sdktrace.TracerProvider {
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.05)),
	)
}

func newMeterExporter(ctx context.Context) *otlpmetrichttp.Exporter {
	exporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithInsecure())
	if err != nil {
		panic(err)
	}
	return exporter
}

func newMeterProvider(resource *resource.Resource, exporter sdkmetric.Exporter) *sdkmetric.MeterProvider {
	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(resource),
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(time.Second)),
		),
	)
}
