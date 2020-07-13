/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"strconv"
	"sync"

	"github.com/zhigui-projects/go-hotstuff/pb"
	"github.com/zhigui-projects/go-hotstuff/transport"
)

func main() {
	var serverAddr string
	var tlsEnabled bool

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
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			resp, err := hsc.Submit(context.Background(), &pb.SubmitRequest{Cmds: []byte(strconv.Itoa(i))})
			if err != nil {
				panic(err)
			}
			fmt.Println("submit resp:", resp)
			wg.Done()
		}(i)
	}
	wg.Wait()
}
