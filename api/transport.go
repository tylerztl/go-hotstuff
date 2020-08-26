/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import "github.com/zhigui-projects/go-hotstuff/protos/pb"

type BroadcastServer interface {
	pb.ConsensusServer
	Broadcast(msg *pb.Message) error
	Unicast(msg *pb.Message, dest int64) error
}

type BroadcastClient interface {
	Recv() (*pb.Message, error)
	Close() error
}
