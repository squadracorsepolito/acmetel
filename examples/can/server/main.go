package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/squadracorsepolito/acmelib"
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

	telemetry.Init(ctx, "can-server-example")

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

// getMessages returns the acmelib representation of the CAN messages
// sent by the client.
func getMessages() []*acmelib.Message {
	// Define the integer message
	sigType, err := acmelib.NewIntegerSignalType("uint8_t", 8, false)
	if err != nil {
		panic(err)
	}

	intMsg := acmelib.NewMessage("my_integer_message", 1000, 8)
	for i := range 8 {
		sig, err := acmelib.NewStandardSignal(fmt.Sprintf("int_signal_%d", i), sigType)
		if err != nil {
			panic(err)
		}

		if err := intMsg.InsertSignal(sig, i*8); err != nil {
			panic(err)
		}
	}

	// Define the enum message
	sigEnum := acmelib.NewSignalEnum("enum")
	for i := range 4 {
		_, err := sigEnum.AddValue(i, fmt.Sprintf("value_%d", i))
		if err != nil {
			panic(err)
		}
	}

	enumMsg := acmelib.NewMessage("my_enum_message", 2000, 4)
	for i := range 4 {
		sig, err := acmelib.NewEnumSignal(fmt.Sprintf("enum_signal_%d", i), sigEnum)
		if err != nil {
			panic(err)
		}

		if err := enumMsg.InsertSignal(sig, i*8); err != nil {
			panic(err)
		}
	}

	return []*acmelib.Message{intMsg, enumMsg}
}
