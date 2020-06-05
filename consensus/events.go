package consensus

import "github.com/zhigui-projects/go-hotstuff/pb"

type EventNotifier interface {
	ExecuteEvent(base *HotStuffBase)
}

type ProposeEvent struct {
	Proposal *pb.Proposal
}

func (p *ProposeEvent) ExecuteEvent(base *HotStuffBase) {
	base.OnProposeEvent(p.Proposal)
	// 出块后，广播区块到其他副本
	go base.BroadcastMsg(&pb.Message{Type: &pb.Message_Proposal{Proposal: p.Proposal}})
}

type ReceiveProposalEvent struct {
	Vote *pb.Vote
}

func (r *ReceiveProposalEvent) ExecuteEvent(base *HotStuffBase) {
	base.PaceMaker.OnReceiveProposal(r.Vote)
	// 接收到Proposal投票后，判断当前节点是不是下一轮的leader，如果是leader，处理投票结果；如果不是leader，发送给下个leader
	go base.DoVote(r.Vote, base.GetLeader())
}

type ReceiveNewViewEvent struct {
	ReplicaId int64
	View      *pb.NewView
}

func (r *ReceiveNewViewEvent) ExecuteEvent(base *HotStuffBase) {
	base.OnReceiveNewView(r.ReplicaId, r.View)
}

type QcFinishEvent struct {
	Proposer int64
	Qc       *pb.QuorumCert
}

func (q *QcFinishEvent) ExecuteEvent(base *HotStuffBase) {
	base.OnQcFinishEvent()
}

type HqcUpdateEvent struct {
	Qc *pb.QuorumCert
}

func (h *HqcUpdateEvent) ExecuteEvent(base *HotStuffBase) {
	base.UpdateQcHigh(h.Qc.ViewNumber, h.Qc)
}

type DecideEvent struct {
	Block *pb.Block
}

func (d *DecideEvent) ExecuteEvent(base *HotStuffBase) {
	// TODO
	logger.Info("consensus complete", "blockHeight", d.Block.Height, "cmds", string(d.Block.Cmds))
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
