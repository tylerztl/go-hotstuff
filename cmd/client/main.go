package main

import (
	"flag"
	"fmt"

	"github.com/zhigui-projects/go-hotstuff/pb"
	"github.com/zhigui-projects/go-hotstuff/transport"
)

func main() {
	var replicaId int64
	var serverAddr string
	var tlsEnabled bool

	flag.Int64Var(&replicaId, "replicaId", 1, "connect hotstuff node replica id")
	flag.StringVar(&serverAddr, "server", "127.0.0.1:8000", "The RPC server to connect to.")
	flag.BoolVar(&tlsEnabled, "tls", false, "Use TLS when communicating with the hotstuff node endpoint")
	flag.Parse()

	client, err := transport.NewBroadcastClient(serverAddr, replicaId, nil)
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}
	defer func() {
		_ = client.Close()
	}()

	done := make(chan struct{})
	go func(msg *pb.Message) {
		if err = client.Send(msg); err != nil {
			panic(err)
		}
	}(&pb.Message{Type: &pb.Message_Proposal{Proposal: &pb.Proposal{Proposer: 1}}})

	<-done
}
