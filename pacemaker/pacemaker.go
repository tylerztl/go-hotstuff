package pacemaker

import (
	"context"

	"github.com/zhigui-projects/go-hotstuff/common/log"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

var logger = log.GetLogger("module", "pacemaker")

type PaceMaker interface {
	Run()

	OnBeat()

	OnNextSyncView()

	OnReceiveNewView()

	GetLeader() int64

	UpdateQcHigh(view int64, qc *pb.QuorumCert)

	Submit(ctx context.Context, req *pb.SubmitRequest) (*pb.SubmitResponse, error)
}
