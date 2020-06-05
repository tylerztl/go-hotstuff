package consensus

import (
	"context"
	"encoding/hex"

	"github.com/zhigui-projects/go-hotstuff/pb"
)

type HotStuffBase struct {
	*HotStuffCore
	*NodeManager
	queue chan MsgExecutor
}

func NewHotStuffBase(id ReplicaID, nodes []*NodeInfo, signer Signer, replicas *ReplicaConf) *HotStuffBase {
	if len(nodes) == 0 {
		logger.Error("not found hotstuff replica node info")
		return nil
	}
	hsb := &HotStuffBase{
		HotStuffCore: NewHotStuffCore(id, signer, replicas),
		queue:        make(chan MsgExecutor),
	}
	nodeMgr := NewNodeManager(id, nodes, hsb)
	hsb.NodeManager = nodeMgr
	return hsb
}

func (hsb *HotStuffBase) handleProposal(proposal *pb.Proposal) {
	if proposal == nil || proposal.Block == nil {
		logger.Warn("handle proposal with empty block")
		return
	}
	logger.Debug("handle proposal", "proposer", proposal.Block.Proposer,
		"height", proposal.Block.Height, "hash", hex.EncodeToString(proposal.Block.SelfQc.BlockHash))

	if err := hsb.OnReceiveProposal(proposal); err != nil {
		logger.Warn("handle proposal catch error", "error", err)
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

func (hsb *HotStuffBase) handleNewView(id ReplicaID, newView *pb.NewView) {
	block, err := hsb.getBlockByHash(newView.GenericQc.BlockHash)
	if err != nil {
		logger.Error("Could not find block of new QC", "error", err)
		return
	}

	hsb.updateHighestQC(block, newView.GenericQc)

	hsb.notify(&ReceiveNewViewEvent{int64(id), newView})
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

func (hsb *HotStuffBase) Start(ctx context.Context) {
	go hsb.StartServer()
	hsb.ConnectWorkers(hsb.queue)

	for {
		select {
		case m := <-hsb.queue:
			m.Execute(hsb)
		case <-ctx.Done():
			return
		}
	}
}

func (hsb *HotStuffBase) receiveMsg(msg *pb.Message, src ReplicaID) {
	logger.Info("received message", "from", src, "to", hsb.GetID(), "msgType", msg.Type)

	switch msg.GetType().(type) {
	case *pb.Message_Proposal:
		hsb.handleProposal(msg.GetProposal())
	case *pb.Message_NewView:
		hsb.handleNewView(src, msg.GetNewView())
	case *pb.Message_Vote:
		hsb.handleVote(msg.GetVote())
	}
}
