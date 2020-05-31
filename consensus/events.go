package consensus

import "github.com/zhigui-projects/go-hotstuff/pb"

//type EventNotifier interface {
//	Execute(base *HotStuffBase)
//}

type ProposeEvent struct {
	Proposal *pb.Proposal
}

type DecideEvent struct {
	Cmds []byte
}

type VoteEvent struct {
	Vote *pb.Vote
}

type ReceiveProposalEvent struct {
	Proposal *pb.Proposal
}

type QcFinishEvent struct {
	Proposer int64
	Qc       *pb.QuorumCert
}

type HqcUpdateEvent struct {
	Qc *pb.QuorumCert
}

type ReceiveNewView struct {
}

//func (p *proposeEvent) Execute(base *HotStuffBase) {
//	base.doBroadcastProposal(p.proposal)
//}

//func (v *VoteEvent) Execute(base *HotStuffBase) {
//	base.doVote(v.vote)
//}

//func (r *ReceiveProposalEvent) Execute(base *HotStuffBase) {
//}

//func (q *QcFinishEvent) Execute(base *HotStuffBase) {
//	if int64(base.GetID()) != base.GetLeader(base.GetVoteHeight()+1) {
//		go base.NextSyncView(base.GetLeader(base.GetVoteHeight() + 1))
//	}
//
//	base.OnPropose(base.Beat())
//}

//func (h *hqcUpdateEvent) Execute(base *HotStuffBase) {
//	base.UpdateHighestQC(h.qc)
//}

type MsgExecutor interface {
	Execute(base *HotStuffBase)
}

type msgEvent struct {
	src ReplicaID
	msg *pb.Message
}

func (m *msgEvent) Execute(base *HotStuffBase) {
	base.receiveMsg(m.msg, m.src)
}
