package consensus

import (
	"github.com/zhigui-projects/go-hotstuff/pacemaker"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

type EventNotifier interface {
	ExecuteEvent(pm pacemaker.PaceMaker)
}

type ProposeEvent struct {
	Proposal *pb.Proposal
}

func (p *ProposeEvent) ExecuteEvent(pm pacemaker.PaceMaker) {
	pm.OnProposeEvent(p.Proposal)
}

type ReceiveProposalEvent struct {
	Proposal *pb.Proposal
	Vote     *pb.Vote
}

func (r *ReceiveProposalEvent) ExecuteEvent(pm pacemaker.PaceMaker) {
	pm.OnReceiveProposal(r.Proposal, r.Vote)
}

type ReceiveNewViewEvent struct {
	ReplicaId int64
	Block     *pb.Block
	View      *pb.NewView
}

func (r *ReceiveNewViewEvent) ExecuteEvent(pm pacemaker.PaceMaker) {
	pm.OnReceiveNewView(r.ReplicaId, r.Block, r.View)
}

type QcFinishEvent struct {
	Proposer int64
	Qc       *pb.QuorumCert
}

func (q *QcFinishEvent) ExecuteEvent(pm pacemaker.PaceMaker) {
	pm.OnQcFinishEvent()
}

type HqcUpdateEvent struct {
	Qc *pb.QuorumCert
}

func (h *HqcUpdateEvent) ExecuteEvent(pm pacemaker.PaceMaker) {
	pm.UpdateQcHigh(h.Qc.ViewNumber, h.Qc)
}

type DecideEvent struct {
	Block *pb.Block
}

func (d *DecideEvent) ExecuteEvent(pm pacemaker.PaceMaker) {
	pm.DoDecide(d.Block)
}

type MsgExecutor interface {
	ExecuteMessage(base *HotStuffBase)
}

type msgEvent struct {
	src ReplicaID
	msg *pb.Message
}

func (m *msgEvent) ExecuteMessage(base *HotStuffBase) {
	base.receiveMsg(m.msg, m.src)
}
