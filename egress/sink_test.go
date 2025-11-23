package egress

import (
	"testing"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal/message"
	"github.com/stretchr/testify/assert"
)

type sinkTestMsg struct {
	call func()
}

func (m *sinkTestMsg) Destroy() {
	m.call()
}

func Test_SinkStage(t *testing.T) {
	assert := assert.New(t)

	msgCount := 32
	conn := connector.NewRingBuffer[*sinkTestMsg](uint32(msgCount))

	stopCh := make(chan struct{})

	destroyCount := 0
	call := func() {
		destroyCount++
		if destroyCount == msgCount {
			close(stopCh)
		}
	}

	for range msgCount {
		msg := message.NewMessage(&sinkTestMsg{
			call: call,
		})

		assert.NoError(conn.Write(msg))
	}

	stage := NewSinkStage(conn)
	assert.NoError(stage.Init(t.Context()))

	go stage.Run(t.Context())

	<-stopCh

	stage.Close()

	assert.Equal(msgCount, destroyCount)
}
