package consensus

import "github.com/zhigui-projects/go-hotstuff/pb"

type EventNotifier interface {
	Execute(base *HotStuffBase)
}

type MsgExecutor interface {
	Execute(base *HotStuffBase)
}

type proposeEvent struct {
	proposal *pb.Proposal
}

func (p *proposeEvent) Execute(base *HotStuffBase) {
	base.doBroadcastProposal(p.proposal)
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
	base.doVote(v.vote)
}

type decideEvent struct {
	cmds []byte
}

func (d *decideEvent) Execute(base *HotStuffBase) {
	base.doDecide(d.cmds)
}

type qcFinishEvent struct {
	qc *pb.QuorumCert
}

func (q *qcFinishEvent) Execute(base *HotStuffBase) {
	if int64(base.GetID()) != base.GetLeader(base.GetVoteHeight()+1) {
		go base.NextSyncView(base.GetLeader(base.GetVoteHeight() + 1))
	}

	base.OnPropose(base.Beat())
}

type hqcUpdateEvent struct {
	qc *pb.QuorumCert
}

func (h *hqcUpdateEvent) Execute(base *HotStuffBase) {
	base.UpdateHighestQC(h.qc)
}

type msgEvent struct {
	src ReplicaID
	msg *pb.Message
}

func (m *msgEvent) Execute(base *HotStuffBase) {
	base.receiveMsg(m.msg, m.src)
}
