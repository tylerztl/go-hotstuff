package consensus

import "github.com/zhigui-projects/go-hotstuff/pb"

type ProposeEvent struct {
	Proposal *pb.Proposal
}

type DecideEvent struct {
	Cmds []byte
}

type VoteEvent struct {
	Vote *pb.Vote
}

type QcFinishEvent struct {
	Proposer int64
	Qc       *pb.QuorumCert
}

type HqcUpdateEvent struct {
	Qc *pb.QuorumCert
}

type NewViewEvent struct {
}

type ReceiveNewView struct {
	ViewNumber int64
	GenericQC  *pb.QuorumCert
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
