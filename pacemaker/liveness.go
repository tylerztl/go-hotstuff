package pacemaker

import (
	"github.com/zhigui-projects/go-hotstuff/consensus"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

type RoundRobinPM struct {
	hsb *consensus.HotStuffBase
}

func (r *RoundRobinPM) Beat(cmd interface{}) {

}

func (r *RoundRobinPM) NextSyncView(viewNumber int64, proposer string) {

}

func (r *RoundRobinPM) ReceiveNewView(viewMsg []byte) {

}

func (r *RoundRobinPM) UpdateHighestQC(qc *pb.QuorumCert) {

}
