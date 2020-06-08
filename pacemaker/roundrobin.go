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
	mut         *sync.Mutex
	submitC     chan []byte
	doneC       chan struct{} // Closes when the consensus halts
	waitTimer   *time.Timer
	hqc         GetHqcFunc
	hqcUpdate   UpdateQCHighFunc
	proposeFunc ProposeFunc
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
		mut:             new(sync.Mutex),
		submitC:         make(chan []byte, 100),
		doneC:           make(chan struct{}),
		waitTimer:       waitTimer,
		hqc:             hqc,
		hqcUpdate:       hqcUpdate,
		proposeFunc:     proposeFunc,
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

func (r *RoundRobinPM) GetLeader(view int64) int64 {
	return (view + 1) % r.metadata.N
}

func (r *RoundRobinPM) OnNextSyncView() {
	leader := r.GetLeader(atomic.LoadInt64(&r.curView))
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
	r.clearViews()
}

func (r *RoundRobinPM) OnProposeEvent(proposal *pb.Proposal) {
	if r.GetLeader(atomic.LoadInt64(&r.curView)) != r.replicaId {
		// 当前节点不是leader，等待下一轮的proposal消息
		r.startNewViewTimer()
	} else {
		// 当前节点是leader，QcFinish处理proposal消息
		//r.stopNewViewTimer()
	}
}

func (r *RoundRobinPM) OnReceiveProposal(proposal *pb.Proposal, vote *pb.Vote) {
	// TODO 副本hqc落后
	if proposal.ViewNumber > atomic.LoadInt64(&r.curView) {
		atomic.StoreInt64(&r.curView, proposal.ViewNumber)
	}

	if r.GetLeader(atomic.LoadInt64(&r.curView)) != r.replicaId {
		// 当前节点不是leader，等待下一轮的proposal消息
		//r.startNewViewTimer()
	} else {
		// 当前节点是leader，QcFinish处理proposal消息
		r.stopNewViewTimer()
	}
}

func (r *RoundRobinPM) OnReceiveNewView(id int64, block *pb.Block, newView *pb.NewView) {
	logger.Info("view status info", "genericView", newView.GetGenericQc().ViewNumber, "hqcView", r.hqc().ViewNumber,
		"viewNumber", newView.ViewNumber, "curView", r.curView)

	if newView.ViewNumber < atomic.LoadInt64(&r.curView) {
		return
	}

	if newView.GetGenericQc().ViewNumber > r.hqc().ViewNumber {
		// 当接收到的hqc比当前hqc高时，如果当前副本没有需要处理的proposal，则直接new-view，无需超时等待
		// TODO block getting
		if block != nil {
			r.hqcUpdate(block, newView.GenericQc)
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
			NewView: &pb.NewView{ViewNumber: atomic.LoadInt64(&r.curView), GenericQc: r.hqc()}}}
		go r.BroadcastMsg(viewMsg)
		r.startNewViewTimer()
		return
	}

	highView := r.addNewViewMsg(id, newView)
	if highView == nil {
		return
	}

	if block != nil {
		r.hqcUpdate(block, newView.GenericQc)
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
	}

	return
}

func (r *RoundRobinPM) clearViews() {
	for key, _ := range r.views {
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

func (r *RoundRobinPM) startNewViewTimer() {
	r.waitTimer.Reset(time.Duration(r.metadata.MsgWaitTimeout) * time.Second)
}

func (r *RoundRobinPM) stopNewViewTimer() {
	r.waitTimer.Stop()
}
