package network

import (
	"runtime/debug"

	"github.com/zhigui-projects/go-hotstuff/common/log"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

var logger = log.GetLogger()

type abServer struct {
	bh *BroadcastHandler
}

func NewABServer() pb.AtomicBroadcastServer {
	return &abServer{
		bh: &BroadcastHandler{},
	}
}

func (a *abServer) Broadcast(srv pb.AtomicBroadcast_BroadcastServer) error {
	logger.Debug("Starting new Broadcast handler")
	defer func() {
		if r := recover(); r != nil {
			logger.Crit("Broadcast client triggered panic", "recover", r, "stack", debug.Stack())
		}
		logger.Debug("Closing Broadcast stream")
	}()
	return a.bh.Handle(srv)
}
