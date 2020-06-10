package pacemaker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/zhigui-projects/go-hotstuff/common/utils"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

type RoundRobinPM struct {
	HotStuff

	replicaId int64
	metadata  *pb.ConfigMetadata
	curView   int64
	views     map[int64]map[int64]*pb.NewView // ViewNumber ~ ReplicaID
	mut       *sync.Mutex
	submitC   chan []byte
	doneC     chan struct{} // Closes when the consensus halts
	waitTimer *time.Timer

	decideExec func(cmds []byte)
}

func NewRoundRobinPM(hs HotStuff, replicaId int64, metadata *pb.ConfigMetadata, decideExec func(cmds []byte)) PaceMaker {
	waitTimer := time.NewTimer(time.Duration(metadata.MsgWaitTimeout) * time.Second)
	waitTimer.Stop()

	return &RoundRobinPM{
		HotStuff:   hs,
		replicaId:  replicaId,
		metadata:   metadata,
		curView:    0,
		views:      make(map[int64]map[int64]*pb.NewView),
		mut:        new(sync.Mutex),
		submitC:    make(chan []byte, 100),
		doneC:      make(chan struct{}),
		waitTimer:  waitTimer,
		decideExec: decideExec,
	}
}

func (r *RoundRobinPM) Run(ctx context.Context) {
	r.ApplyPaceMaker(r)
	go r.Start(ctx)

	go r.OnBeat()

	for {
		select {
		case <-r.waitTimer.C:
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
		// 节点启动后第一次处理交易，通过new-view唤醒其他副本节点
		if !r.isLeader() {
			go func() {
				r.submitC <- s
			}()
			// OnReceiveNewView
			atomic.AddInt64(&r.curView, 1)
			viewMsg := &pb.Message{Type: &pb.Message_NewView{
				NewView: &pb.NewView{ViewNumber: atomic.LoadInt64(&r.curView), GenericQc: r.GetHighQC()}}}
			go r.BroadcastMsg(viewMsg)
			r.startNewViewTimer()
			return
		}

		if err := r.OnPropose(atomic.LoadInt64(&r.curView), r.GetHighQC().BlockHash, s); err != nil {
			logger.Error("propose catch error", "error", err)
		}
	}
}

func (r *RoundRobinPM) GetLeader(view int64) int64 {
	return (view + 1) % r.metadata.N
}

func (r *RoundRobinPM) isLeader() bool {
	return atomic.LoadInt64(&r.curView)%r.metadata.N == r.replicaId
}

func (r *RoundRobinPM) OnNextSyncView() {
	leader := r.GetLeader(atomic.LoadInt64(&r.curView))
	atomic.AddInt64(&r.curView, 1)
	logger.Debug("enter next view", "view", atomic.LoadInt64(&r.curView), "leader", leader)

	if leader != r.replicaId {
		// TODO 逻辑待推敲
		if !r.GetConnectStatus(leader) {
			r.OnNextSyncView()
			return
		}

		r.startNewViewTimer()
		viewMsg := &pb.Message{Type: &pb.Message_NewView{
			NewView: &pb.NewView{ViewNumber: atomic.LoadInt64(&r.curView), GenericQc: r.GetHighQC()}}}
		_ = r.UnicastMsg(viewMsg, leader)
	} else {
		if len(r.submitC) > 0 {
			// 当leader == r.GetID()时，由 ReceiveNewView 事件来触发 propose, 因为可能需要updateHighestQC
			viewMsg := &pb.NewView{ViewNumber: atomic.LoadInt64(&r.curView), GenericQc: r.GetHighQC()}
			r.OnReceiveNewView(r.replicaId, nil, viewMsg)
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
	go r.BroadcastMsg(&pb.Message{Type: &pb.Message_Proposal{Proposal: proposal}})
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
	go r.DoVote(r.GetLeader(proposal.ViewNumber), vote)
}

// TODO 并发量大时偶尔会存在超时现象，待分析
func (r *RoundRobinPM) OnReceiveNewView(id int64, block *pb.Block, newView *pb.NewView) {
	logger.Info("view status info", "genericView", newView.GetGenericQc().ViewNumber, "hqcView", r.GetHighQC().ViewNumber,
		"viewNumber", newView.ViewNumber, "curView", r.curView)

	if newView.ViewNumber < atomic.LoadInt64(&r.curView) {
		return
	}

	if newView.GetGenericQc().ViewNumber > r.GetHighQC().ViewNumber {
		// 当接收到的hqc比当前hqc高时，如果当前副本没有需要处理的proposal，则直接new-view，无需超时等待
		// TODO block getting
		if block != nil {
			r.UpdateHighestQC(block, newView.GenericQc)
		}
		r.UpdateQcHigh(newView.ViewNumber, newView.GenericQc)
		if len(r.submitC) > 0 {
			go r.OnBeat()
		} else {
			go r.OnNextSyncView()
		}
		return
	} else if newView.ViewNumber > atomic.LoadInt64(&r.curView) {
		// 副本间同步view number
		r.stopNewViewTimer()
		atomic.StoreInt64(&r.curView, newView.ViewNumber)
		viewMsg := &pb.Message{Type: &pb.Message_NewView{
			NewView: &pb.NewView{ViewNumber: atomic.LoadInt64(&r.curView), GenericQc: r.GetHighQC()}}}
		go r.BroadcastMsg(viewMsg)
		r.startNewViewTimer()
		return
	}

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
	for key := range r.views {
		if key < atomic.LoadInt64(&r.curView) {
			delete(r.views, key)
		}
	}
}

func (r *RoundRobinPM) OnQcFinishEvent() {
	if r.GetLeader(atomic.LoadInt64(&r.curView)) == r.replicaId {
		atomic.AddInt64(&r.curView, 1)
		logger.Debug("enter next view", "view", atomic.LoadInt64(&r.curView), "leader", r.GetLeader(atomic.LoadInt64(&r.curView)))
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
	logger.Info("consensus complete", "blockHeight", block.Height, "cmds", string(block.Cmds))
	r.decideExec(block.Cmds)
}

func (r *RoundRobinPM) startNewViewTimer() {
	r.waitTimer.Reset(time.Duration(r.metadata.MsgWaitTimeout) * time.Second)
}

func (r *RoundRobinPM) stopNewViewTimer() {
	r.waitTimer.Stop()
}
