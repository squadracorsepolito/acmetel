package stage

import (
	"github.com/squadracorsepolito/acmetel/internal/message"
	"github.com/squadracorsepolito/acmetel/internal/pool"
)

type msg = message.Message

type processorWPtr[W, WArgs, MIn, MOut any] = pool.ProcessorWorkerPtr[W, WArgs, MIn, MOut]
type egressWorkerPtr[W, WArgs, M any] = pool.EgressWorkerPtr[W, WArgs, M]
