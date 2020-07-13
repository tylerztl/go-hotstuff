/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"

	"github.com/zhigui-projects/go-hotstuff/pb"
)

type HotStuff interface {
	BroadcastServer
	Signer

	Start(ctx context.Context)
	ApplyPaceMaker(pm PaceMaker)
	OnPropose(curView int64, parentHash, cmds []byte) error
	OnProposalVote(vote *pb.Vote) error
	UpdateHighestQC(block *pb.Block, qc *pb.QuorumCert)
	GetHighQC() *pb.QuorumCert
	GetVoteHeight() int64
	LoadBlock(hash []byte) (*pb.Block, error)
	GetConnectStatus(id int64) bool
}
