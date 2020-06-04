package pacemaker

import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/zhigui-projects/go-hotstuff/consensus"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

type RoundRobinPM struct {
	*consensus.HotStuffBase
	curView   int64
	submitC   chan []byte
	waitTimer *time.Timer
}

func NewRoundRobinPM(hsb *consensus.HotStuffBase) *RoundRobinPM {
	rr := &RoundRobinPM{
		HotStuffBase: hsb,
		curView:      0,
		submitC:      make(chan []byte, 10),
		waitTimer:    time.NewTimer(3 * time.Second),
	}
	rr.waitTimer.Stop()
	pb.RegisterHotstuffServer(rr.Server(), rr)
	return rr
}

func (r *RoundRobinPM) Run() {
	go r.Start(context.Background())
	go r.handleEvent()
	if r.GetID() == 0 {
		go r.OnBeat()
	}
}

func (r *RoundRobinPM) handleEvent() {
	for {
		select {
		case n := <-r.GetNotifier():
			switch n.(type) {
			case *consensus.ProposeEvent:
				if r.GetLeader() != r.GetID() {
					// 当前节点不是leader，等待下一轮的proposal消息
					r.startNewViewTimer()
				} else {
					// 当前节点是leader，处理proposal消息
					r.stopNewViewTimer()
				}
				go r.DoBroadcastProposal(n.(*consensus.ProposeEvent).Proposal)
			case *consensus.ReceiveProposalEvent:
				if r.GetLeader() != r.GetID() {
					// 当前节点不是leader，等待下一轮的proposal消息
					r.startNewViewTimer()
				} else {
					// TODO 多节点投票收集
					// 当前节点是leader，处理proposal消息
					r.stopNewViewTimer()
				}
				go r.DoVote(n.(*consensus.ReceiveProposalEvent).Vote, r.GetLeader())
			case *consensus.HqcUpdateEvent:
				v := n.(*consensus.HqcUpdateEvent)
				r.UpdateQcHigh(v.Qc.ViewNumber, v.Qc)
			case *consensus.ReceiveNewViewEvent:
				r.OnReceiveNewView(n.(*consensus.ReceiveNewViewEvent).View)
			case *consensus.QcFinishEvent:
				if r.GetID() == r.GetLeader() && len(r.submitC) > 0 {
					// 与接收到new-view信息重叠
					go r.OnBeat()
				} else {
					go r.OnNextSyncView()
				}
			case *consensus.DecideEvent:
				go r.doDecide(n.(*consensus.DecideEvent).Block)
			}
		case <-r.waitTimer.C:
			go r.OnNextSyncView()
		}
	}
}

func (r *RoundRobinPM) doDecide(block *pb.Block) {
	cmds, _ := strconv.Atoi(string(block.Cmds))
	logger.Info("consensus complete", "blockHeight", block.Height, "cmds", cmds)
}

func (r *RoundRobinPM) OnNextSyncView() {
	r.stopNewViewTimer()
	leader := r.GetLeader()
	atomic.AddInt64(&r.curView, 1)
	logger.Debug("enter next view", "view", atomic.LoadInt64(&r.curView), "leader", leader)

	if leader != r.GetID() {
		r.startNewViewTimer()
		viewMsg := &pb.Message{Type: &pb.Message_NewView{
			NewView: &pb.NewView{ViewNumber: atomic.LoadInt64(&r.curView), GenericQc: r.GetHighQC()}}}
		_ = r.UnicastMsg(viewMsg, leader)
	} else {
		if len(r.submitC) > 0 {
			// 当leader == r.GetID()时，由 ReceiveNewView 事件来触发 propose, 因为可能需要updateHighestQC
			//go r.OnBeat()
		} else {
			go r.OnNextSyncView()
		}
	}
}

func (r *RoundRobinPM) UpdateQcHigh(viewNumber int64, qc *pb.QuorumCert) {
	if viewNumber > atomic.LoadInt64(&r.curView) {
		atomic.StoreInt64(&r.curView, viewNumber)
	}
}

func (r *RoundRobinPM) OnReceiveNewView(view *pb.NewView) {
	r.UpdateQcHigh(view.ViewNumber, view.GenericQc)

	// TODO 处理多个节点发送的NEW-VIEW
	if len(r.submitC) > 0 {
		r.stopNewViewTimer()
		go r.OnBeat()
	} else {
		r.startNewViewTimer()
	}
}

func (r *RoundRobinPM) GetLeader() int64 {
	return (atomic.LoadInt64(&r.curView) + 1) % int64(len(r.Nodes))
}

func (r *RoundRobinPM) Submit(ctx context.Context, req *pb.SubmitRequest) (*pb.SubmitResponse, error) {
	logger.Debug("receive new submit request", "cmds", string(req.Cmds))
	if len(req.Cmds) == 0 {
		return &pb.SubmitResponse{Status: pb.Status_BAD_REQUEST}, errors.New("request data is empty")
	}

	r.submitC <- req.Cmds

	return &pb.SubmitResponse{Status: pb.Status_SUCCESS}, nil
}

func (r *RoundRobinPM) OnBeat() {
	select {
	case s := <-r.submitC:
		if err := r.OnPropose(atomic.LoadInt64(&r.curView), r.GetHighQC().BlockHash, s); err != nil {
			logger.Error("propose catch error", "error", err)
		}
	}
}

func (r *RoundRobinPM) startNewViewTimer() {
	r.waitTimer.Reset(3 * time.Second)
}

func (r *RoundRobinPM) stopNewViewTimer() {
	r.waitTimer.Stop()
}
