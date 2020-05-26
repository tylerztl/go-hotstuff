package consensus

import (
	"context"
	"crypto"

	"github.com/zhigui-projects/go-hotstuff/pacemaker"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

type HotStuffBase struct {
	*HotStuffCore
	pacemaker.PaceMaker
	queue    chan MsgExecutor
	execFunc func(cmds []byte)
	nodes    *NodeManager
}

func NewHotStuffBase(id ReplicaID, nodes []*NodeInfo, signer crypto.Signer, replicas *ReplicaConf) *HotStuffBase {
	if len(nodes) == 0 {
		logger.Error("not found hotstuff replica node info")
		return nil
	}
	return &HotStuffBase{
		HotStuffCore: NewHotStuffCore(id, signer, replicas),
		PaceMaker:    pacemaker.NewRoundRobinPM(),
		queue:        make(chan MsgExecutor),
		execFunc:     func([]byte) {},
		nodes:        NewNodeManager(id, nodes),
	}
}

func (hsb *HotStuffBase) handlePropose(proposal *pb.Proposal) {
	if proposal.Block == nil {
		logger.Warn("handle propose with empty block", "proposer", proposal.Proposer)
		return
	}

	if err := hsb.OnReceiveProposal(proposal.Block); err != nil {
		logger.Warn("handle propose catch error", "error", err)
		return
	}
}

func (hsb *HotStuffBase) handleVote(vote *pb.Vote) {
	if ok := hsb.replicas.VerifyVote(vote); !ok {
		return
	}
	if err := hsb.OnReceiveVote(vote); err != nil {
		logger.Warn("handle vote catch error", "error", err)
		return
	}
}

func (hsb *HotStuffBase) handleNewView(newView *pb.NewView) {

}

func (hsb *HotStuffBase) doBroadcastProposal(proposal *pb.Proposal) {

}

func (hsb *HotStuffBase) doVote(vote *pb.Vote) {

}

func (hsb *HotStuffBase) doDecide(cmds []byte) {
	hsb.execFunc(cmds)
}

func (hsb *HotStuffBase) GetID() ReplicaID {
	return hsb.id
}

func (hsb *HotStuffBase) GetVoteHeight() int64 {
	return hsb.voteHeight
}

func (hsb *HotStuffBase) Start(ctx context.Context) {
	go hsb.nodes.StartServer()
	hsb.nodes.ConnectWorkers(hsb.queue)

	for {
		select {
		case m := <-hsb.queue:
			m.Execute(hsb)
		case n := <-hsb.GetNotifier():
			n.Execute(hsb)
		case <-ctx.Done():
			return
		}
	}
}

func (hsb *HotStuffBase) receiveMsg(msg *pb.Message, src ReplicaID) {
	logger.Debug("received message", "from", src, "to", hsb.id, "msgType", msg.Type)

	switch msg.GetType().(type) {
	case *pb.Message_Proposal:
		hsb.handlePropose(msg.GetProposal())
	case *pb.Message_NewView:
		hsb.handleNewView(msg.GetNewView())
	case *pb.Message_Vote:
		hsb.handleVote(msg.GetVote())
	}
}
