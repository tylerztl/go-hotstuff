package consensus

import "github.com/zhigui-projects/go-hotstuff/pb"

type EventNotifier interface {
	Execute(base *HotStuffBase)
}

type proposeEvent struct {
	proposal *pb.Proposal
}

func (p *proposeEvent) Execute(base *HotStuffBase) {
}

type receiveProposalEvent struct {
	proposal *pb.Proposal
}

func (r *receiveProposalEvent) Execute(base *HotStuffBase) {

}

type voteEvent struct {
	vote *pb.Vote
}

func (v *voteEvent) Execute(base *HotStuffBase) {

}

type decideEvent struct {
	cmds []byte
}

func (d *decideEvent) Execute(base *HotStuffBase) {

}

type qcFinishEvent struct {
	qc *pb.QuorumCert
}

func (q *qcFinishEvent) Execute(base *HotStuffBase) {

}

type hqcUpdateEvent struct {
	qc *pb.QuorumCert
}

func (h *hqcUpdateEvent) Execute(base *HotStuffBase) {

}
