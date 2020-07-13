/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"

	"github.com/zhigui-projects/go-hotstuff/pb"
)

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
	OnReceiveNewView(id int64, newView *pb.NewView)
	// 收集到n-f个proposal vote事件
	OnQcFinishEvent(qc *pb.QuorumCert)
	// 区块完成Decide阶段，达成共识，执行交易
	DoDecide(block *pb.Block)
	// highest qc 更新事件
	UpdateQcHigh(viewNumber int64, qc *pb.QuorumCert)
}
