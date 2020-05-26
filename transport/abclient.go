package transport

import (
	"context"

	"github.com/pkg/errors"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

type BroadcastClient interface {
	Send(msg *pb.Message) error
	Recv() (*pb.Message, error)
	Close() error
}

type NodeContext struct {
	replicaId key
}
type key int64

var nodeKey key

func NewContext(ctx context.Context, n *NodeContext) context.Context {
	return context.WithValue(ctx, nodeKey, n)
}

type abClient struct {
	client pb.AtomicBroadcast_BroadcastClient
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
	bc, err := pb.NewAtomicBroadcastClient(conn).Broadcast(NewContext(context.Background(), &NodeContext{key(replicaId)}))
	if err != nil {
		return nil, err
	}

	return &abClient{client: bc}, nil
}

func (a *abClient) Recv() (*pb.Message, error) {
	msg, err := a.client.Recv()
	if err != nil {
		logger.Error("broadcast client recv msg failed", "error", err)
		return nil, err
	}
	return msg, nil
}

func (a *abClient) Send(msg *pb.Message) error {
	if err := a.client.Send(msg); err != nil {
		logger.Error("broadcast client send msg failed", "error", err)
		return err
	}
	return nil
}

func (a *abClient) Close() error {
	return a.client.CloseSend()
}
