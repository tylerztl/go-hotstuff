package transport

import (
	"context"
	"strconv"
	"sync"

	"github.com/pkg/errors"
	"github.com/zhigui-projects/go-hotstuff/common/log"
	"github.com/zhigui-projects/go-hotstuff/pb"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

type BroadcastServer interface {
	pb.AtomicBroadcastServer
	BroadcastMsg(msg *pb.Message) error
	UnicastMsg(msg *pb.Message, dest int64) error
}

type abServer struct {
	sendChan map[int64]chan<- *pb.Message
	sendLock *sync.RWMutex
	logger   log.Logger
}

func NewABServer() BroadcastServer {
	return &abServer{
		sendChan: make(map[int64]chan<- *pb.Message),
		sendLock: new(sync.RWMutex),
		logger:   log.GetLogger("module", "transport"),
	}
}

func (a *abServer) Broadcast(srv pb.AtomicBroadcast_BroadcastServer) error {
	addr, src := extractRemoteAddress(srv.Context())
	a.logger.Debug("Starting new broadcast handler for remote peer", "addr", addr, "replicaId", src)

	ch := make(chan *pb.Message)
	a.sendLock.Lock()
	if oldChan, ok := a.sendChan[src]; ok {
		a.logger.Debug("create new connection from replica node", "replicaId", src)
		close(oldChan)
	}
	a.sendChan[src] = ch
	a.sendLock.Unlock()

	// TODO: firstly connect sync data

	var err error
	for msg := range ch {
		if err = srv.Send(msg); err != nil {
			a.sendLock.Lock()
			delete(a.sendChan, src)
			a.sendLock.Unlock()
			a.logger.Error("disconnected with replica node", "replicaId", src, "error", err)
		}
	}

	return err
}

func (a *abServer) BroadcastMsg(msg *pb.Message) error {
	a.sendLock.RLock()
	defer a.sendLock.RUnlock()

	for _, ch := range a.sendChan {
		ch <- msg
	}
	return nil
}

func (a *abServer) UnicastMsg(msg *pb.Message, dest int64) error {
	a.sendLock.RLock()
	defer a.sendLock.RUnlock()

	ch, ok := a.sendChan[dest]
	if !ok {
		a.logger.Error("unicast msg to invalid replica node", "replicaId", dest)
		return errors.Errorf("unicast msg to invalid replica node: %d", dest)
	}
	ch <- msg
	return nil
}

func extractRemoteAddress(ctx context.Context) (remoteAddress string, replicaId int64) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		// 注意key小写
		if value, ok := md["replicaid"]; ok {
			id, _ := strconv.Atoi(value[0])
			replicaId = int64(id)
		}
	}

	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", 0
	}
	if address := p.Addr; address != nil {
		remoteAddress = address.String()
	}
	return
}
