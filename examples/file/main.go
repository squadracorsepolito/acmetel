package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/squadracorsepolito/acmetel"
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/egress"
	"github.com/squadracorsepolito/acmetel/ingress"
	"github.com/squadracorsepolito/acmetel/processor"
)

const connectorSize = 2048

func main() {
	ctx, cancelCtx := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancelCtx()

	fileIngressToCustom := connector.NewRingBuffer[*ingress.FileMessage](connectorSize)
	customToFileEgress := connector.NewRingBuffer[*ingress.FileMessage](connectorSize)

	fileIngressCfg := ingress.DefaultFileConfig()
	fileIngressCfg.WatchedDirs = []string{"./data/in"}
	fileIngressStage := ingress.NewFileStage(fileIngressToCustom, fileIngressCfg)

	customCfg := processor.DefaultCustomConfig(acmetel.StageRunningModeSingle)
	customCfg.Name = "file_to_file"
	customStage := processor.NewCustomStage(newFileHandler(), fileIngressToCustom, customToFileEgress, customCfg)

	fileEgressCfg := egress.DefaultFileConfig("./data/out/out.txt")
	fileEgressStage := egress.NewFileStage(customToFileEgress, fileEgressCfg)

	pipeline := acmetel.NewPipeline()

	pipeline.AddStage(fileIngressStage)
	pipeline.AddStage(customStage)
	pipeline.AddStage(fileEgressStage)

	if err := pipeline.Init(ctx); err != nil {
		panic(err)
	}

	go pipeline.Run(ctx)
	defer pipeline.Close()

	<-ctx.Done()
}
