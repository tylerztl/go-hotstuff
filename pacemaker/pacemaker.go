package pacemaker

import (
	"context"

	"github.com/zhigui-projects/go-hotstuff/common/log"
	"github.com/zhigui-projects/go-hotstuff/pb"
	"github.com/zhigui-projects/go-hotstuff/transport"
)

var logger = log.GetLogger("module", "pacemaker")

type HotStuff interface {
	transport.BroadcastServer

	Start(ctx context.Context)
	ApplyPaceMaker(pm PaceMaker)
	OnPropose(curView int64, parentHash, cmds []byte) error
	DoVote(leader int64, vote *pb.Vote)
	UpdateHighestQC(block *pb.Block, qc *pb.QuorumCert)
	GetHighQC() *pb.QuorumCert
	GetConnectStatus(id int64) bool
}

type PaceMaker interface {
	// 启动pacemaker
	Run(ctx context.Context)
	// 提交待执行cmds到pacemaker
	Submit(cmds []byte) error
	// 触发执行cmds
	OnBeat()
	// 获取下一个view的leader
	GetLeader(view int64) int64
	// 触发view change
	OnNextSyncView()
	// 监听到共识返回的提交proposal的事件
	OnProposeEvent(proposal *pb.Proposal)
	// 监听到接收到其他节点发来的proposal消息的事件
	OnReceiveProposal(proposal *pb.Proposal, vote *pb.Vote)
	// 监听到接收到其他节点发来的new view消息的事件
	OnReceiveNewView(id int64, block *pb.Block, newView *pb.NewView)
	// 收集到n-f个proposal vote事件
	OnQcFinishEvent()
	// 区块完成Decide阶段，达成共识，执行交易
	DoDecide(block *pb.Block)
	// highest qc 更新事件
	UpdateQcHigh(viewNumber int64, qc *pb.QuorumCert)
}
