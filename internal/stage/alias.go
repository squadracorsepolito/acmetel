package stage

import (
	"github.com/squadracorsepolito/acmetel/internal/message"
	"github.com/squadracorsepolito/acmetel/internal/pool"
)

type msg = message.Message
type reOrdMsg = message.ReOrderable

type ingressWorkerPtr[W, WArgs, M any] = pool.IngressWorkerPtr[W, WArgs, M]
type handlerWorkerPtr[W, WArgs, MIn, MOut any] = pool.HandlerWorkerPtr[W, WArgs, MIn, MOut]
type egressWorkerPtr[W, WArgs, M any] = pool.EgressWorkerPtr[W, WArgs, M]
