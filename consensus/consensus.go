package consensus

import (
	"context"
	"time"

	"github.com/zhigui-projects/go-hotstuff/pb"
)

type HotStuffBase struct {
	*HotStuffCore
	*NodeManager
	queue   chan MsgExecutor
	waitMsg chan struct{}
}

func NewHotStuffBase(id ReplicaID, nodes []*NodeInfo, signer Signer, replicas *ReplicaConf) *HotStuffBase {
	if len(nodes) == 0 {
		logger.Error("not found hotstuff replica node info")
		return nil
	}
	hsb := &HotStuffBase{
		HotStuffCore: NewHotStuffCore(id, signer, replicas),
		queue:        make(chan MsgExecutor),
		waitMsg:      make(chan struct{}),
	}
	nodeMgr := NewNodeManager(id, nodes, hsb)
	hsb.NodeManager = nodeMgr
	return hsb
}

func (hsb *HotStuffBase) handleProposal(proposal *pb.Proposal) {
	logger.Debug("handle proposal msg", "proposer", proposal.Proposer, "blockHeight", proposal.Block.Height)
	if proposal.Block == nil {
		logger.Warn("handle propose with empty block", "proposer", proposal.Proposer)
		return
	}
	hsb.waitMsg <- struct{}{}

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
	block, err := hsb.getBlockByHash(newView.GenericQc.BlockHash)
	if err != nil {
		logger.Error("Could not find block of new QC", "error", err)
		return
	}
	hsb.waitMsg <- struct{}{}
	hsb.updateHighestQC(block, newView.GenericQc)
	hsb.notify(&ReceiveNewView{ViewNumber: newView.ViewNumber, GenericQC: newView.GenericQc})
}

func (hsb *HotStuffBase) DoBroadcastProposal(proposal *pb.Proposal) {
	_ = hsb.BroadcastMsg(&pb.Message{Type: &pb.Message_Proposal{Proposal: proposal}})
}

func (hsb *HotStuffBase) DoVote(vote *pb.Vote, leader int64) {
	if leader != hsb.GetID() {
		_ = hsb.UnicastMsg(&pb.Message{Type: &pb.Message_Vote{Vote: vote}}, leader)
	} else {
		_ = hsb.OnReceiveVote(vote)
	}
}

func (hsb *HotStuffBase) GetID() int64 {
	return int64(hsb.id)
}

func (hsb *HotStuffBase) Start(ctx context.Context) {
	go hsb.StartServer()
	hsb.ConnectWorkers(hsb.queue)
	go hsb.newViewTimeout()

	for {
		select {
		case m := <-hsb.queue:
			m.Execute(hsb)
		case <-ctx.Done():
			return
		}
	}
}

func (hsb *HotStuffBase) newViewTimeout() {
	for {
		select {
		case <-hsb.waitMsg:
		case <-time.After(time.Second * 3):
			hsb.notify(&NewViewEvent{})
		}
	}
}

func (hsb *HotStuffBase) receiveMsg(msg *pb.Message, src ReplicaID) {
	logger.Debug("received message", "from", src, "to", hsb.GetID(), "msgType", msg.Type)

	switch msg.GetType().(type) {
	case *pb.Message_Proposal:
		hsb.handleProposal(msg.GetProposal())
	case *pb.Message_NewView:
		hsb.handleNewView(msg.GetNewView())
	case *pb.Message_Vote:
		hsb.handleVote(msg.GetVote())
	}
}
