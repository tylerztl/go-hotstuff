/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pacemaker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/clock"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/zhigui-projects/go-hotstuff/api"
	"github.com/zhigui-projects/go-hotstuff/common/log"
	"github.com/zhigui-projects/go-hotstuff/common/utils"
	"github.com/zhigui-projects/go-hotstuff/protos/pb"
)

type RoundRobinPM struct {
	api.HotStuff

	replicaId  int64
	metadata   *pb.ConfigMetadata
	curView    int64
	views      map[int64]map[int64]*pb.NewView // ViewNumber ~ ReplicaID
	submitC    chan []byte
	doneC      chan struct{} // Closes when the consensus halts
	waitTimer  clock.Timer
	decideExec func(cmds []byte)
	mut        sync.Mutex
	logger     api.Logger
}

func NewRoundRobinPM(hs api.HotStuff, replicaId int64, metadata *pb.ConfigMetadata, decideExec func(cmds []byte)) api.PaceMaker {
	waitTimer := clock.NewClock().NewTimer(time.Duration(metadata.MsgWaitTimeout) * time.Second)
	waitTimer.Stop()

	return &RoundRobinPM{
		HotStuff:   hs,
		replicaId:  replicaId,
		metadata:   metadata,
		curView:    0,
		views:      make(map[int64]map[int64]*pb.NewView),
		submitC:    make(chan []byte, 100),
		doneC:      make(chan struct{}),
		waitTimer:  waitTimer,
		decideExec: decideExec,
		logger:     log.GetLogger("module", "pacemaker", "node", replicaId),
	}
}

func (r *RoundRobinPM) Run(ctx context.Context) {
	r.ApplyPaceMaker(r)
	go r.Start(ctx)

	go r.OnBeat()

	for {
		select {
		case <-r.waitTimer.C():
			go r.OnNextSyncView()
		}
	}
}

func (r *RoundRobinPM) Submit(cmds []byte) error {
	select {
	case r.submitC <- cmds:
	case <-r.doneC:
		return errors.Errorf("pacemaker is stopped")
	}
	return nil
}

func (r *RoundRobinPM) OnBeat() {
	select {
	case s := <-r.submitC:
		// 节点启动后第一次处理交易，如果不是leader, proposal转发给当前leader
		leader := r.GetCurLeader(atomic.LoadInt64(&r.curView))
		if leader != r.replicaId {
			go func() {
				if err := r.Unicast(&pb.Message{Type: &pb.Message_Forward{Forward: &pb.Forward{Data: s}}}, leader); err != nil {
					r.logger.Error("OnBeat: msg forward failed", "to", leader, "err", err)
				}
			}()
			r.startNewViewTimer()
		} else {
			if err := r.OnPropose(atomic.LoadInt64(&r.curView), r.GetHighQC().BlockHash, s); err != nil {
				r.logger.Error("propose catch error", "error", err)
			}
		}
	}
}

func (r *RoundRobinPM) GetLeader(view int64) int64 {
	return (view + 1) % r.metadata.N
}

func (r *RoundRobinPM) GetCurLeader(viewNumber int64) int64 {
	return viewNumber % r.metadata.N
}

func (r *RoundRobinPM) OnNextSyncView() {
	leader := r.GetLeader(atomic.LoadInt64(&r.curView))
	atomic.AddInt64(&r.curView, 1)
	r.logger.Debug("enter next sync view", "view", atomic.LoadInt64(&r.curView), "leader", leader)

	if leader != r.replicaId {
		// TODO 逻辑待推敲
		if !r.GetConnectStatus(leader) {
			r.OnNextSyncView()
			return
		}

		r.startNewViewTimer()
		viewMsg, err := r.createSignedViewChange()
		if err != nil {
			r.logger.Error("create signed view change msg failed when unicast next new view msg", "error", err)
			return
		}
		_ = r.Unicast(viewMsg, leader)
	} else {
		if len(r.submitC) > 0 {
			// 当leader == r.GetID()时，由 ReceiveNewView 事件来触发 propose, 因为可能需要updateHighestQC
			viewMsg := &pb.NewView{ViewNumber: atomic.LoadInt64(&r.curView), GenericQc: r.GetHighQC()}
			r.OnReceiveNewView(r.replicaId, viewMsg)
		} else {
			r.startNewViewTimer()
		}
	}
	r.clearViews()
}

func (r *RoundRobinPM) OnProposeEvent(proposal *pb.Proposal) {
	if r.GetLeader(atomic.LoadInt64(&r.curView)) != r.replicaId {
		// 当前节点不是leader，等待下一轮的proposal消息
		r.startNewViewTimer()
		// TODO 逻辑待推敲
		if !r.GetConnectStatus(r.GetLeader(atomic.LoadInt64(&r.curView))) {
			atomic.AddInt64(&r.curView, 1)
			proposal.ViewNumber = atomic.LoadInt64(&r.curView)
		}
	} else {
		// 当前节点是leader，QcFinish处理proposal消息
		//r.stopNewViewTimer()
	}

	// 出块后，广播区块到其他副本
	go r.Broadcast(&pb.Message{Type: &pb.Message_Proposal{Proposal: proposal}})
}

func (r *RoundRobinPM) OnReceiveProposal(proposal *pb.Proposal, vote *pb.Vote) {
	// TODO 副本hqc落后
	if proposal.ViewNumber > atomic.LoadInt64(&r.curView) {
		atomic.StoreInt64(&r.curView, proposal.ViewNumber)
	}

	if r.GetLeader(atomic.LoadInt64(&r.curView)) != r.replicaId {
		// 当前节点不是leader，等待下一轮的proposal消息
		//r.startNewViewTimer()
		// TODO 逻辑待推敲
		if !r.GetConnectStatus(r.GetLeader(atomic.LoadInt64(&r.curView))) {
			atomic.AddInt64(&r.curView, 1)
			proposal.ViewNumber = atomic.LoadInt64(&r.curView)
			r.OnReceiveProposal(proposal, vote)
		}
	} else {
		// 当前节点是leader，QcFinish处理proposal消息
		r.stopNewViewTimer()
	}

	// 接收到Proposal投票后，判断当前节点是不是下一轮的leader，如果是leader，处理投票结果；如果不是leader，发送给下个leader
	leader := r.GetLeader(proposal.ViewNumber)
	if leader != r.replicaId {
		if err := r.Unicast(&pb.Message{Type: &pb.Message_Vote{Vote: vote}}, leader); err != nil {
			r.logger.Error("unicast proposal vote msg error failed", "to", leader, "err", err)
		}
	} else {
		if err := r.OnProposalVote(vote); err != nil {
			r.logger.Error("proposal vote msg failed", "to", leader, "err", err)
		}
	}
}

// TODO 并发量大时偶尔会存在超时现象，待分析
func (r *RoundRobinPM) OnReceiveNewView(id int64, newView *pb.NewView) {
	if newView.ViewNumber < atomic.LoadInt64(&r.curView) {
		return
	}

	r.logger.Debug("receive new view msg", "genericView", newView.GetGenericQc().ViewNumber,
		"hqcView", r.GetHighQC().ViewNumber, "viewNumber", newView.ViewNumber, "curView", r.curView)

	// TODO Proposal Message可能在NewView Message之后到达, 可能造成分叉
	block, err := r.AsyncWaitBlock(newView.GetGenericQc().BlockHash)
	if err != nil {
		r.logger.Error("receive new view load generic block failed", "error", err)
		return
	}

	r.mut.Lock()
	if newView.GetGenericQc().ViewNumber > r.GetHighQC().ViewNumber {
		// 当接收到的hqc比当前hqc高时，如果当前副本没有需要处理的proposal，则直接new-view，无需超时等待
		if block != nil {
			r.UpdateHighestQC(block, newView.GenericQc)
		}
		r.UpdateQcHigh(newView.ViewNumber, newView.GenericQc)
		if len(r.submitC) > 0 {
			go r.OnBeat()
		} else {
			go r.OnNextSyncView()
		}
		r.mut.Unlock()
		return
	} else if newView.ViewNumber > atomic.LoadInt64(&r.curView) {
		// 副本间同步view number
		r.stopNewViewTimer()
		atomic.StoreInt64(&r.curView, newView.ViewNumber)
		viewMsg, err := r.createSignedViewChange()
		if err != nil {
			r.mut.Unlock()
			r.logger.Error("create signed view change msg failed when sync new view number", "error", err)
			return
		}
		r.mut.Unlock()
		go r.Broadcast(viewMsg)
		r.startNewViewTimer()
		return
	}
	r.mut.Unlock()

	highView := r.addNewViewMsg(id, newView)
	if highView == nil {
		return
	}

	if block != nil {
		r.UpdateHighestQC(block, newView.GenericQc)
	}
	r.UpdateQcHigh(highView.ViewNumber, highView.GenericQc)

	if len(r.submitC) > 0 {
		r.stopNewViewTimer()
		go r.OnBeat()
	} else {
		r.startNewViewTimer()
	}
}

func (r *RoundRobinPM) createSignedViewChange() (*pb.Message, error) {
	view := &pb.NewView{ViewNumber: atomic.LoadInt64(&r.curView), GenericQc: r.GetHighQC()}
	data, err := proto.Marshal(view)
	if err != nil {
		return nil, err
	}
	sig, err := r.Sign(data)
	if err != nil {
		return nil, err
	}

	return &pb.Message{Type: &pb.Message_ViewChange{ViewChange: &pb.ViewChange{Data: data, Signature: sig}}}, nil
}

func (r *RoundRobinPM) addNewViewMsg(id int64, newView *pb.NewView) (highView *pb.NewView) {
	r.mut.Lock()
	defer r.mut.Unlock()

	if views, ok := r.views[newView.ViewNumber]; ok {
		if len(views) >= utils.GetQuorumSize(r.metadata) {
			return
		}
		if view, ok := views[id]; ok {
			if view.GenericQc.ViewNumber > newView.GenericQc.ViewNumber {
				views[id] = newView
			}
		} else {
			views[id] = newView
			if len(views) == utils.GetQuorumSize(r.metadata) {
				for _, v := range views {
					if highView == nil {
						highView = v
					} else if v.ViewNumber > highView.ViewNumber {
						highView = v
					}
				}
			}
		}
	} else {
		r.views[newView.ViewNumber] = make(map[int64]*pb.NewView)
		r.views[newView.ViewNumber][id] = newView
		if 1 == utils.GetQuorumSize(r.metadata) {
			highView = newView
		}
	}

	return
}

func (r *RoundRobinPM) clearViews() {
	r.mut.Lock()
	defer r.mut.Unlock()

	for key := range r.views {
		if key < atomic.LoadInt64(&r.curView) {
			delete(r.views, key)
		}
	}
}

func (r *RoundRobinPM) OnQcFinishEvent(qc *pb.QuorumCert) {
	if qc.ViewNumber > atomic.LoadInt64(&r.curView) {
		atomic.StoreInt64(&r.curView, qc.ViewNumber)
	}
	if r.GetLeader(atomic.LoadInt64(&r.curView)) == r.replicaId {
		atomic.AddInt64(&r.curView, 1)
		r.logger.Debug("enter next view", "view", atomic.LoadInt64(&r.curView), "leader", r.GetLeader(atomic.LoadInt64(&r.curView)))
		if len(r.submitC) > 0 {
			go r.OnBeat()
		} else {
			go r.OnNextSyncView()
		}
	} else {
		go r.OnNextSyncView()
	}
}

func (r *RoundRobinPM) UpdateQcHigh(viewNumber int64, qc *pb.QuorumCert) {
	if viewNumber > atomic.LoadInt64(&r.curView) {
		atomic.StoreInt64(&r.curView, viewNumber)
	}
}

func (r *RoundRobinPM) DoDecide(block *pb.Block) {
	r.logger.Debug("consensus complete", "blockHeight", block.Height, "cmds", string(block.Cmds))
	r.decideExec(block.Cmds)
}

func (r *RoundRobinPM) startNewViewTimer() {
	r.stopNewViewTimer()
	r.waitTimer.Reset(time.Duration(r.metadata.MsgWaitTimeout) * time.Second)
}

func (r *RoundRobinPM) stopNewViewTimer() {
	r.waitTimer.Stop()
}
