/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package consensus

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/zhigui-projects/go-hotstuff/api"
	"github.com/zhigui-projects/go-hotstuff/common/log"
	"github.com/zhigui-projects/go-hotstuff/pb"
	"google.golang.org/grpc/peer"
)

type HotStuffBase struct {
	api.PaceMaker
	*HotStuffCore
	*NodeManager
	queue  chan MsgExecutor
	logger api.Logger
}

func NewHotStuffBase(id ReplicaID, nodes []*NodeInfo, signer api.Signer, replicas *ReplicaConf) *HotStuffBase {
	logger := log.GetLogger("module", "consensus", "node", id)
	if len(nodes) == 0 {
		logger.Error("not found hotstuff replica node info")
		return nil
	}

	hsb := &HotStuffBase{
		HotStuffCore: NewHotStuffCore(id, signer, replicas, logger),
		NodeManager:  NewNodeManager(id, nodes, logger),
		queue:        make(chan MsgExecutor),
		logger:       logger,
	}
	pb.RegisterHotstuffServer(hsb.Server(), hsb)
	return hsb
}

func (hsb *HotStuffBase) ApplyPaceMaker(pm api.PaceMaker) {
	hsb.PaceMaker = pm
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
	hsb.logger.Info("receive new submit request", "remoteAddress", remoteAddress)
	if len(req.Cmds) == 0 {
		return &pb.SubmitResponse{Status: pb.Status_BAD_REQUEST}, errors.New("request data is empty")
	}

	if err := hsb.PaceMaker.Submit(req.Cmds); err != nil {
		return &pb.SubmitResponse{Status: pb.Status_SERVICE_UNAVAILABLE}, err
	} else {
		return &pb.SubmitResponse{Status: pb.Status_SUCCESS}, nil
	}
}

func (hsb *HotStuffBase) handleMessage(src ReplicaID, msg *pb.Message) {
	hsb.logger.Debug("received message", "from", src, "msg", msg.String())

	switch msg.GetType().(type) {
	case *pb.Message_Proposal:
		hsb.handleProposal(msg.GetProposal())
	case *pb.Message_ViewChange:
		hsb.handleViewChange(src, msg.GetViewChange())
	case *pb.Message_Vote:
		hsb.handleVote(src, msg.GetVote())
	case *pb.Message_Forward:
		hsb.handleForward(src, msg.GetForward())
	default:
		hsb.logger.Error("received invalid message type", "from", src, "msg", msg.String())
	}
}

// TODO 封装on接口
func (hsb *HotStuffBase) handleProposal(proposal *pb.Proposal) {
	if proposal.Block == nil {
		hsb.logger.Error("receive invalid proposal msg with empty block")
		return
	}

	if err := hsb.HotStuffCore.OnReceiveProposal(proposal); err != nil {
		hsb.logger.Error("handle proposal msg catch error", "error", err)
	}
}

func (hsb *HotStuffBase) handleViewChange(id ReplicaID, vc *pb.ViewChange) {
	if ok := hsb.GetReplicas().VerifyIdentity(id, vc.Signature, vc.Data); !ok {
		hsb.logger.Error("receive invalid view change msg signature verify failed")
		return
	}
	newView := &pb.NewView{}
	if err := proto.Unmarshal(vc.Data, newView); err != nil {
		hsb.logger.Error("receive invalid view change msg that unmarshal NewView failed", "from", id, "error", err)
		return
	}
	if newView.GenericQc == nil {
		hsb.logger.Error("receive invalid view change msg that genericQc is null", "from", id)
		return
	}

	if err := hsb.OnNewView(int64(id), newView); err != nil {
		hsb.logger.Error("handle view change msg catch error", "from", id, "error", err)
		return
	}
}

func (hsb *HotStuffBase) handleVote(id ReplicaID, vote *pb.Vote) {
	if len(vote.BlockHash) == 0 || vote.Cert == nil {
		hsb.logger.Error("receive invalid vote msg data", "from", id)
		return
	}
	if ok := hsb.GetReplicas().VerifyIdentity(id, vote.Cert.Signature, vote.BlockHash); !ok {
		hsb.logger.Error("receive invalid vote msg signature verify failed")
		return
	}
	if int64(id) != vote.Voter {
		hsb.logger.Error("receive invalid vote msg replica id not match", "from", id, "voter", vote.Voter)
		return
	}
	if leader := hsb.GetLeader(vote.ViewNumber); ReplicaID(leader) != hsb.id {
		hsb.logger.Error("receive invalid vote msg not match leader id", "from", id,
			"view", vote.ViewNumber, "leader", leader)
		return
	}

	if err := hsb.OnProposalVote(vote); err != nil {
		hsb.logger.Error("handle vote msg catch error", "from", id, "error", err)
		return
	}
}

func (hsb *HotStuffBase) handleForward(id ReplicaID, msg *pb.Forward) {
	_ = hsb.PaceMaker.Submit(msg.Data)
}
