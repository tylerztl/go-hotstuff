package consensus

import "github.com/zhigui-projects/go-hotstuff/pb"

type ProposeEvent struct {
	Proposal *pb.Proposal
}

type ReceiveProposalEvent struct {
	Vote *pb.Vote
}

type DecideEvent struct {
	Block *pb.Block
}

type QcFinishEvent struct {
	Proposer int64
	Qc       *pb.QuorumCert
}

type HqcUpdateEvent struct {
	Qc *pb.QuorumCert
}

type ReceiveNewViewEvent struct {
	View *pb.NewView
}

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
