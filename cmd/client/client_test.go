/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/zhigui-projects/go-hotstuff/consensus"
	"github.com/zhigui-projects/go-hotstuff/protos/pb"
	"github.com/zhigui-projects/go-hotstuff/transport"
)

var nodes = []*consensus.NodeInfo{
	{
		Id:      0,
		Addr:    "127.0.0.1:8000",
		TlsOpts: nil,
	},
	{
		Id:      1,
		Addr:    "127.0.0.1:8001",
		TlsOpts: nil,
	},
	{
		Id:      2,
		Addr:    "127.0.0.1:8002",
		TlsOpts: nil,
	},
	{
		Id:      3,
		Addr:    "127.0.0.1:8003",
		TlsOpts: nil,
	},
}

func TestSubmit(t *testing.T) {
	client, err := transport.NewGrpcClient(nil)
	if err != nil {
		t.Fatal(err)
	}
	hscs := make([]pb.HotstuffClient, len(nodes))
	for i, node := range nodes {
		conn, err := client.NewConnection(node.Addr)
		if err != nil {
			t.Fatal(err)
		}

		hscs[i] = pb.NewHotstuffClient(conn)
	}

	var wg sync.WaitGroup
	//var length int64 = 1024
	for i := 0; i < 100; i++ {
		j := i % len(nodes)
		wg.Add(1)
		go func(i int) {
			//buf := make([]byte, length)
			//if _, err := io.ReadFull(rand.Reader, buf); err != nil {
			//	panic(err)
			//}
			resp, err := hscs[j].Submit(context.Background(), &pb.SubmitRequest{Cmds: []byte(strconv.Itoa(i))})
			if err != nil {
				panic(err)
			}
			t.Log("submit resp:", resp)
			wg.Done()
		}(i)
	}
	wg.Wait()
}
