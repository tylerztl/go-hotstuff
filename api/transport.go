/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import "github.com/zhigui-projects/go-hotstuff/pb"

type BroadcastServer interface {
	pb.AtomicBroadcastServer
	BroadcastMsg(msg *pb.Message) error
	UnicastMsg(msg *pb.Message, dest int64) error
}

type BroadcastClient interface {
	Send(msg *pb.Message) error
	Recv() (*pb.Message, error)
	Close() error
}
