package network

import (
	"context"

	"github.com/pkg/errors"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

type BroadcastClient interface {
	Send(msg *pb.Message) error
	GetAck() error
	Close() error
}

type abClient struct {
	client pb.AtomicBroadcast_BroadcastClient
}

// NewBroadcastClient creates a simple instance of the BroadcastClient interface
func NewBroadcastClient(address string, opts *TLSOptions) (BroadcastClient, error) {
	client, err := NewGrpcClient(opts)
	if err != nil {
		return nil, err
	}
	conn, err := client.NewConnection(address)
	if err != nil {
		return nil, errors.Errorf("grpc client failed to connect to %s, err: %v", address, err)
	}
	bc, err := pb.NewAtomicBroadcastClient(conn).Broadcast(context.TODO())
	if err != nil {
		return nil, err
	}

	return &abClient{client: bc}, nil
}

func (a *abClient) GetAck() error {
	msg, err := a.client.Recv()
	if err != nil {
		return err
	}
	if msg.Status != pb.Status_SUCCESS {
		return errors.Errorf("got unexpected status: %v -- %s", msg.Status, msg.Info)
	}
	return nil
}

func (a *abClient) Send(msg *pb.Message) error {
	if err := a.client.Send(msg); err != nil {
		return errors.WithMessage(err, "could not send")
	}
	return nil
	//return a.GetAck()
}

func (a *abClient) Close() error {
	return a.client.CloseSend()
}
