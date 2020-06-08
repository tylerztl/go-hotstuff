package pacemaker

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/zhigui-projects/go-hotstuff/common/utils"
	"github.com/zhigui-projects/go-hotstuff/pb"
	"github.com/zhigui-projects/go-hotstuff/transport"
)

type RoundRobinPM struct {
	transport.BroadcastServer

	replicaId   int64
	metadata    *pb.ConfigMetadata
	curView     int64
	views       map[int64]map[int64]*pb.NewView // ViewNumber ~ ReplicaID
	submitC     chan []byte
	doneC       chan struct{} // Closes when the consensus halts
	waitTimer   *time.Timer
	hqc         GetHqcFunc
	hqcUpdate   UpdateQCHighFunc
	proposeFunc ProposeFunc
	mut         *sync.Mutex
}

func NewRoundRobinPM(replicaId int64, metadata *pb.ConfigMetadata, hqc GetHqcFunc, hqcUpdate UpdateQCHighFunc,
	proposeFunc ProposeFunc, bs transport.BroadcastServer) *RoundRobinPM {
	waitTimer := time.NewTimer(time.Duration(metadata.MsgWaitTimeout) * time.Second)
	waitTimer.Stop()

	return &RoundRobinPM{
		BroadcastServer: bs,
		replicaId:       replicaId,
		metadata:        metadata,
		curView:         0,
		views:           make(map[int64]map[int64]*pb.NewView),
		submitC:         make(chan []byte, 10),
		doneC:           make(chan struct{}),
		waitTimer:       waitTimer,
		hqc:             hqc,
		hqcUpdate:       hqcUpdate,
		proposeFunc:     proposeFunc,
		mut:             new(sync.Mutex),
	}
}

func (r *RoundRobinPM) Run() {
	// bootstrap node
	if r.replicaId == 0 {
		go r.OnBeat()
	}

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
		if err := r.proposeFunc(atomic.LoadInt64(&r.curView), r.hqc().BlockHash, s); err != nil {
			logger.Error("propose catch error", "error", err)
		}
	}
}

func (r *RoundRobinPM) GetLeader() int64 {
	return (atomic.LoadInt64(&r.curView) + 1) % r.metadata.N
}

func (r *RoundRobinPM) OnNextSyncView() {
	r.stopNewViewTimer()
	leader := r.GetLeader()
	atomic.AddInt64(&r.curView, 1)
	logger.Debug("enter next view", "view", atomic.LoadInt64(&r.curView), "leader", leader)

	if leader != r.replicaId {
		r.startNewViewTimer()
		viewMsg := &pb.Message{Type: &pb.Message_NewView{
			NewView: &pb.NewView{ViewNumber: atomic.LoadInt64(&r.curView), GenericQc: r.hqc()}}}
		_ = r.UnicastMsg(viewMsg, leader)
	} else {
		if len(r.submitC) > 0 {
			// 当leader == r.GetID()时，由 ReceiveNewView 事件来触发 propose, 因为可能需要updateHighestQC
			viewMsg := &pb.NewView{ViewNumber: atomic.LoadInt64(&r.curView), GenericQc: r.hqc()}
			r.OnReceiveNewView(r.replicaId, nil, viewMsg)
		} else {
			r.startNewViewTimer()
		}
	}
}

func (r *RoundRobinPM) OnProposeEvent(proposal *pb.Proposal) {
	if r.GetLeader() != r.replicaId {
		// 当前节点不是leader，等待下一轮的proposal消息
		r.startNewViewTimer()
	} else {
		// 当前节点是leader，QcFinish处理proposal消息
		r.stopNewViewTimer()
	}
}

func (r *RoundRobinPM) OnReceiveProposal(vote *pb.Vote) {
	if vote.ViewNumber > atomic.LoadInt64(&r.curView) {
		atomic.StoreInt64(&r.curView, vote.ViewNumber)
	}

	if r.GetLeader() != r.replicaId {
		// 当前节点不是leader，等待下一轮的proposal消息
		r.startNewViewTimer()
	} else {
		// TODO 多节点投票收集
		// 当前节点是leader，处理proposal消息
		r.stopNewViewTimer()
	}
}

func (r *RoundRobinPM) OnReceiveNewView(id int64, block *pb.Block, newView *pb.NewView) {
	logger.Info("view info", "genericView", newView.GetGenericQc().ViewNumber, "hqcView", r.hqc().ViewNumber,
		"viewNumber", newView.ViewNumber, "curView", r.curView)

	if newView.ViewNumber < atomic.LoadInt64(&r.curView) {
		return
	}

	if newView.GetGenericQc().ViewNumber > r.hqc().ViewNumber {
		if block != nil {
			r.hqcUpdate(block, newView.GenericQc)
		}
		r.UpdateQcHigh(newView.ViewNumber, newView.GenericQc)
		if len(r.submitC) > 0 {
			r.stopNewViewTimer()
			go r.OnBeat()
		} else {
			go r.OnNextSyncView()
		}
		return
	} else if newView.ViewNumber > atomic.LoadInt64(&r.curView) {
		// 副本间同步view number
		r.startNewViewTimer()
		atomic.StoreInt64(&r.curView, newView.ViewNumber)
		viewMsg := &pb.Message{Type: &pb.Message_NewView{
			NewView: &pb.NewView{ViewNumber: atomic.LoadInt64(&r.curView), GenericQc: r.hqc()}}}
		_ = r.BroadcastMsg(viewMsg)
		return
	}

	r.mut.Lock()
	var highView *pb.NewView
	if views, ok := r.views[newView.ViewNumber]; ok {
		if len(views) >= utils.GetQuorumSize(r.metadata) {
			r.mut.Unlock()
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
	}

	if highView == nil {
		r.mut.Unlock()
		return
	}

	if block != nil {
		r.hqcUpdate(block, newView.GenericQc)
	}

	r.UpdateQcHigh(highView.ViewNumber, highView.GenericQc)
	r.mut.Unlock()

	if len(r.submitC) > 0 {
		r.stopNewViewTimer()
		go r.OnBeat()
	} else {
		r.startNewViewTimer()
	}
}

func (r *RoundRobinPM) OnQcFinishEvent() {
	if r.replicaId == r.GetLeader() {
		atomic.AddInt64(&r.curView, 1)
		if len(r.submitC) > 0 {
			r.stopNewViewTimer()
			// view更新在hqc
			go r.OnBeat()
		} else {
			logger.Debug("enter next view", "view", atomic.LoadInt64(&r.curView), "leader", r.GetLeader())
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

func (r *RoundRobinPM) startNewViewTimer() {
	r.waitTimer.Reset(time.Duration(r.metadata.MsgWaitTimeout) * time.Second)
}

func (r *RoundRobinPM) stopNewViewTimer() {
	r.waitTimer.Stop()
}
