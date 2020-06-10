package consensus

import (
	"context"
	"encoding/hex"

	"github.com/pkg/errors"
	"github.com/zhigui-projects/go-hotstuff/pacemaker"
	"github.com/zhigui-projects/go-hotstuff/pb"
	"google.golang.org/grpc/peer"
)

type HotStuffBase struct {
	pacemaker.PaceMaker
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
		NodeManager:  NewNodeManager(id, nodes),
		queue:        make(chan MsgExecutor),
	}
	pb.RegisterHotstuffServer(hsb.Server(), hsb)
	return hsb
}

func (hsb *HotStuffBase) ApplyPaceMaker(pm pacemaker.PaceMaker) {
	hsb.PaceMaker = pm
}

func (hsb *HotStuffBase) handleProposal(proposal *pb.Proposal) {
	if proposal == nil || proposal.Block == nil {
		logger.Warn("handle proposal with empty block")
		return
	}
	logger.Debug("handle proposal", "proposer", proposal.Block.Proposer,
		"height", proposal.Block.Height, "hash", hex.EncodeToString(proposal.Block.SelfQc.BlockHash))

	if err := hsb.HotStuffCore.OnReceiveProposal(proposal); err != nil {
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

	//hsb.UpdateHighestQC(block, newView.GenericQc)

	hsb.notify(&ReceiveNewViewEvent{int64(id), block, newView})
}

func (hsb *HotStuffBase) DoVote(leader int64, vote *pb.Vote) {
	if leader != hsb.GetID() {
		if err := hsb.UnicastMsg(&pb.Message{Type: &pb.Message_Vote{Vote: vote}}, leader); err != nil {
			logger.Error("do vote error when unicast msg", "to", leader)
		}
	} else {
		if err := hsb.OnReceiveVote(vote); err != nil {
			logger.Warn("do vote error when receive vote", "to", leader)
		}
	}
}

func (hsb *HotStuffBase) Start(ctx context.Context) {
	if hsb.PaceMaker == nil {
		panic("pacemaker is nil, please set pacemaker")
	}

	go hsb.StartServer()
	go hsb.ConnectWorkers(hsb.queue)

	for {
		select {
		case m := <-hsb.queue:
			go m.ExecuteMessage(hsb)
		case n := <-hsb.GetNotifier():
			go n.ExecuteEvent(hsb.PaceMaker)
		case <-ctx.Done():
			return
		}
	}
}

func (hsb *HotStuffBase) Submit(ctx context.Context, req *pb.SubmitRequest) (*pb.SubmitResponse, error) {
	var remoteAddress string
	if p, _ := peer.FromContext(ctx); p != nil {
		if address := p.Addr; address != nil {
			remoteAddress = address.String()
		}
	}
	logger.Debug("receive new submit request", "cmds", string(req.Cmds), "remoteAddress", remoteAddress)
	if len(req.Cmds) == 0 {
		return &pb.SubmitResponse{Status: pb.Status_BAD_REQUEST}, errors.New("request data is empty")
	}

	if err := hsb.PaceMaker.Submit(req.Cmds); err != nil {
		return &pb.SubmitResponse{Status: pb.Status_SERVICE_UNAVAILABLE}, err
	} else {
		return &pb.SubmitResponse{Status: pb.Status_SUCCESS}, nil
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
