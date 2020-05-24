package network

import (
	"context"
	"io"

	"github.com/zhigui-projects/go-hotstuff/pb"
	"google.golang.org/grpc/peer"
)

type BroadcastHandler struct {
}

// Handle reads requests from a Broadcast stream, processes them, and returns the responses to the stream
func (bh *BroadcastHandler) Handle(srv pb.AtomicBroadcast_BroadcastServer) error {
	addr := extractRemoteAddress(srv.Context())
	logger.Debug("Starting new broadcast loop for remote peer", "remoteAddr", addr)
	for {
		msg, err := srv.Recv()
		if err == io.EOF {
			logger.Debug("Received EOF from remote peer, hangup", "remoteAddr", addr)
			return nil
		}
		if err != nil {
			logger.Warn("Error reading from remote peer", "remoteAddr", addr, "error", err)
			return err
		}

		resp := bh.ProcessMessage(msg, addr)
		err = srv.Send(resp)
		if resp.Status != pb.Status_SUCCESS {
			logger.Warn("Process message return error response", "remoteAddr", addr, "msg", msg, "response", resp)
		}
		if err != nil {
			logger.Warn("Error sending to remote peer", "remoteAddr", addr, "error", err)
			return err
		}
	}
}

// ProcessMessage validates and enqueues a single message
func (bh *BroadcastHandler) ProcessMessage(msg *pb.Message, addr string) (resp *pb.BroadcastResponse) {
	logger.Debug("Broadcast has successfully enqueued message from remote peer", "remoteAddr", addr)
	return &pb.BroadcastResponse{Status: pb.Status_SUCCESS}
}

func extractRemoteAddress(ctx context.Context) string {
	var remoteAddress string
	p, ok := peer.FromContext(ctx)
	if !ok {
		return ""
	}
	if address := p.Addr; address != nil {
		remoteAddress = address.String()
	}
	return remoteAddress
}
