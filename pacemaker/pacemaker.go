package pacemaker

import (
	"github.com/zhigui-projects/go-hotstuff/common/log"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

var logger = log.GetLogger("module", "pacemaker")

type PaceMaker interface {
	Beat() (parentHash, cmds []byte)

	NextSyncView(leader int64)

	ReceiveNewView(viewMsg []byte)

	UpdateHighestQC(qc *pb.QuorumCert)

	GetLeader(voteHeight int64) int64
}
