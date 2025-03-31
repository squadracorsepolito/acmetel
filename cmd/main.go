package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/squadracorsepolito/acmetel"
	"github.com/squadracorsepolito/acmetel/cannelloni"
	"github.com/squadracorsepolito/acmetel/core"
	"github.com/squadracorsepolito/acmetel/internal"
)

func main() {
	ctx, cancelCtx := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancelCtx()

	rb := internal.NewRingBuffer[[]byte](1024)

	server := acmetel.NewServerUDP(rb, acmetel.NewDefaultServerUDPConfig())

	cannelloniPreprocessor := cannelloni.NewPreprocessor(rb)

	msgCh := make(chan *core.Message, 32)

	wg := sync.WaitGroup{}
	wg.Add(3)

	go func() {
		cannelloniPreprocessor.Run(ctx, msgCh)
		wg.Done()
	}()

	go func() {
	loop:
		for {
			select {
			case <-ctx.Done():
				break loop

			case msg := <-msgCh:
				log.Print(msg)
			}
		}

		wg.Done()
	}()

	go func() {
		if err := server.Run(ctx); err != nil {
			log.Print(err)
		}
		wg.Done()
	}()

	<-ctx.Done()

	close(msgCh)
	wg.Wait()
}
