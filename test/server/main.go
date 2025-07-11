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
	"github.com/squadracorsepolito/acmetel/handler"
	"github.com/squadracorsepolito/acmetel/ingress"
	"github.com/squadracorsepolito/acmetel/message"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

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

	udpToCannelloni := connector.NewRingBuffer[*message.UDPPayload](16_000)
	cannelloniToCAN := connector.NewRingBuffer[*message.RawCANMessageBatch](16_000)
	canToQuestDB := connector.NewRingBuffer[*message.CANSignalBatch](16_000)

	udpCfg := ingress.NewDefaultUDPConfig()
	udpIngress := ingress.NewUDP(udpCfg)
	udpIngress.SetOutput(udpToCannelloni)

	cannelloniCfg := handler.NewDefaultCannelloniConfig()
	cannelloniHandler := handler.NewCannelloni(cannelloniCfg)
	cannelloniHandler.SetInput(udpToCannelloni)
	cannelloniHandler.SetOutput(cannelloniToCAN)

	canCfg := handler.NewDefaultCANConfig()
	canCfg.Messages = getMessages()
	canHandler := handler.NewCAN(canCfg)
	canHandler.SetInput(cannelloniToCAN)
	canHandler.SetOutput(canToQuestDB)

	questDBCfg := egress.NewDefaultQuestDBConfig()
	questDBCfg.MaxWorkers = 32
	questDBCfg.QueueDepthPerWorker = 1
	questDBEgress := egress.NewQuestDB(questDBCfg)
	questDBEgress.SetInput(canToQuestDB)

	pipeline := acmetel.NewPipeline()

	pipeline.AddStage(udpIngress)
	pipeline.AddStage(cannelloniHandler)
	pipeline.AddStage(canHandler)
	pipeline.AddStage(questDBEgress)

	if err := pipeline.Init(ctx); err != nil {
		panic(err)
	}

	go pipeline.Run(ctx)
	defer pipeline.Stop()

	<-ctx.Done()
}

func getMessages() []*acmelib.Message {
	messages := []*acmelib.Message{}

	sigType, _ := acmelib.NewIntegerSignalType("sig_type", 8, false)
	for i := range 113 {
		msg := acmelib.NewMessage(fmt.Sprintf("message_%d", i), acmelib.MessageID(i), 8)

		for j := range 8 {
			sig, _ := acmelib.NewStandardSignal(fmt.Sprintf("message_%d_signal_%d", i, j), sigType)

			if err := msg.InsertSignal(sig, j*8); err != nil {
				panic(err)
			}
		}

		messages = append(messages, msg)
	}

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
