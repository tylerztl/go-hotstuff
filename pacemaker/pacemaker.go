package pacemaker

import "github.com/zhigui-projects/go-hotstuff/pb"

type PaceMaker interface {
	Beat() (parentHash, cmds []byte)

	NextSyncView(leader int64)

	ReceiveNewView(viewMsg []byte)

	UpdateHighestQC(qc *pb.QuorumCert)

	GetLeader(voteHeight int64) int64
}
