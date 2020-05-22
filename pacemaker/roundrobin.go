package pacemaker

import (
	"github.com/zhigui-projects/go-hotstuff/pb"
)

type RoundRobinPM struct {
	submitC chan []byte
}

func NewRoundRobinPM() *RoundRobinPM {
	return &RoundRobinPM{
		submitC: make(chan []byte),
	}
}

func (r *RoundRobinPM) Beat() (parentHash, cmds []byte) {

}

func (r *RoundRobinPM) NextSyncView(leader int64) {

}

func (r *RoundRobinPM) ReceiveNewView(viewMsg []byte) {

}

func (r *RoundRobinPM) UpdateHighestQC(qc *pb.QuorumCert) {

}

func (r *RoundRobinPM) GetLeader(voteHeight int64) int64 {

}
