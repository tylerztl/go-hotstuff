package transport

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	"github.com/zhigui-projects/go-hotstuff/common/log"
	"github.com/zhigui-projects/go-hotstuff/pb"
	"google.golang.org/grpc/metadata"
)

type BroadcastClient interface {
	Send(msg *pb.Message) error
	Recv() (*pb.Message, error)
	Close() error
}

type abClient struct {
	client pb.AtomicBroadcast_BroadcastClient
	logger log.Logger
}

// NewBroadcastClient creates a simple instance of the BroadcastClient interface
func NewBroadcastClient(address string, replicaId int64, opts *TLSOptions) (BroadcastClient, error) {
	client, err := NewGrpcClient(opts)
	if err != nil {
		return nil, err
	}
	conn, err := client.NewConnection(address)
	if err != nil {
		return nil, errors.Errorf("grpc client failed to connect to %s, err: %v", address, err)
	}

	// 在 grpc 中，client 与 server 之间通过 context 传递上下文数据的时候，不能使用 context.WithValue。
	md := metadata.Pairs("replicaid", strconv.Itoa(int(replicaId)))
	bc, err := pb.NewAtomicBroadcastClient(conn).Broadcast(metadata.NewOutgoingContext(context.Background(), md))
	if err != nil {
		return nil, err
	}

	return &abClient{client: bc, logger: log.GetLogger("module", "transport")}, nil
}

func (a *abClient) Recv() (*pb.Message, error) {
	msg, err := a.client.Recv()
	if err != nil {
		a.logger.Error("broadcast client recv msg failed", "error", err)
		return nil, err
	}
	return msg, nil
}

func (a *abClient) Send(msg *pb.Message) error {
	if err := a.client.Send(msg); err != nil {
		a.logger.Error("broadcast client send msg failed", "error", err)
		return err
	}
	return nil
}

func (a *abClient) Close() error {
	return a.client.CloseSend()
}
