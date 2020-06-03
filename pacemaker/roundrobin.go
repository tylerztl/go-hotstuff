package pacemaker

import (
	"context"
	"strconv"
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
				go r.DoBroadcastProposal(n.(*consensus.ProposeEvent).Proposal)
				go r.StartNewViewTimer()
			case *consensus.ReceiveProposalEvent:
				if r.GetLeader() != r.GetID() {
					r.StartNewViewTimer()
				} else {
					r.StopNewViewTimer()
				}
				go r.DoVote(n.(*consensus.ReceiveProposalEvent).Vote, r.GetLeader())
			case *consensus.HqcUpdateEvent:
				if n.(*consensus.HqcUpdateEvent).Qc.ViewNumber > r.curView {
					r.curView = n.(*consensus.HqcUpdateEvent).Qc.ViewNumber
				}
			case *consensus.ReceiveNewViewEvent:
				// TODO 处理多个节点发送的NEW-VIEW
				if n.(*consensus.ReceiveNewViewEvent).View.ViewNumber > r.curView {
					r.curView = n.(*consensus.ReceiveNewViewEvent).View.ViewNumber
				}
				if len(r.submitC) > 0 {
					r.StopNewViewTimer()
					go r.OnBeat()
				} else {
					r.StartNewViewTimer()
				}
			case *consensus.QcFinishEvent:
				if r.GetID() == r.GetLeader() {
					if len(r.submitC) > 0 {
						go r.OnBeat()
					} else {
						go r.OnNextSyncView()
					}
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
	go r.StopNewViewTimer()
	leader := r.GetLeader()
	r.curView++
	logger.Debug("enter next view", "view", r.curView, "leader", leader)
	// 当leader == r.GetID()时，由 ReceiveNewView 事件来触发 propose, 因为可能需要updateHighestQC
	if leader != r.GetID() {
		go r.StartNewViewTimer()
		viewMsg := &pb.Message{Type: &pb.Message_NewView{
			NewView: &pb.NewView{ViewNumber: r.curView, GenericQc: r.GetHighQC()}}}
		_ = r.UnicastMsg(viewMsg, leader)
	} else {
		if len(r.submitC) > 0 {
			go r.OnBeat()
		} else {
			go r.OnNextSyncView()
		}
	}
}

func (r *RoundRobinPM) GetLeader() int64 {
	return (r.curView + 1) % int64(len(r.Nodes))
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
		if err := r.OnPropose(r.curView, r.GetHighQC().BlockHash, s); err != nil {
			logger.Error("propose catch error", "error", err)
		}
	}
}

func (r *RoundRobinPM) StartNewViewTimer() {
	r.waitTimer.Reset(3 * time.Second)
}

func (r *RoundRobinPM) StopNewViewTimer() {
	r.waitTimer.Stop()
}
