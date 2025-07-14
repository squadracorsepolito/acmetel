package stage

import (
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/worker"
)

type msg = internal.Message
type reOrderableMsg = internal.ReOrderableMessage

type ingressWorkerPtr[W, WArgs, M any] = worker.IngressWorkerPtr[W, WArgs, M]
type handlerWorkerPtr[W, WArgs, MIn, MOut any] = worker.HandlerWorkerPtr[W, WArgs, MIn, MOut]
type egressWorkerPtr[W, WArgs, M any] = worker.EgressWorkerPtr[W, WArgs, M]
