package main

import (
	"context"
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

	client, err := transport.NewGrpcClient(nil)
	if err != nil {
		panic(err)
	}
	conn, err := client.NewConnection(serverAddr)
	if err != nil {
		panic(err)
	}

	hsc := pb.NewHotstuffClient(conn)
	resp, err := hsc.Submit(context.Background(), &pb.SubmitRequest{Cmds: []byte("hello")})
	if err != nil {
		panic(err)
	}
	fmt.Println("submit resp:", resp)
}
