package main

import (
	"flag"
	"fmt"

	"github.com/zhigui-projects/go-hotstuff/network"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

func main() {
	var serverAddr string
	var tlsEnabled bool

	flag.StringVar(&serverAddr, "server", "127.0.0.1:8000", "The RPC server to connect to.")
	flag.BoolVar(&tlsEnabled, "tls", false, "Use TLS when communicating with the hotstuff node endpoint")
	flag.Parse()

	client, err := network.NewBroadcastClient(serverAddr, nil)
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}
	defer func() {
		_ = client.Close()
	}()

	done := make(chan struct{})
	go func(msg *pb.Message) {
		go func() {
			if err = client.GetAck(); err != nil {
				fmt.Println("GetAck catch error:", err)
			}
			close(done)
		}()

		if err = client.Send(msg); err != nil {
			panic(err)
		}
	}(&pb.Message{Type: &pb.Message_Proposal{Proposal: &pb.Proposal{Proposer: 1}}})

	<-done
}
