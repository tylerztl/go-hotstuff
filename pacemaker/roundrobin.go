package pacemaker

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/zhigui-projects/go-hotstuff/consensus"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

type RoundRobinPM struct {
	*consensus.HotStuffBase
	curView int64
	submitC chan []byte
}

func NewRoundRobinPM(hsb *consensus.HotStuffBase) *RoundRobinPM {
	rr := &RoundRobinPM{
		HotStuffBase: hsb,
		submitC:      make(chan []byte),
	}
	pb.RegisterHotstuffServer(rr.Server(), rr)
	return rr
}

func (r *RoundRobinPM) Run() {
	r.Start(context.Background())

	go r.OnBeat()

	go r.handleEvent()
}

func (r *RoundRobinPM) handleEvent() {
	notifier := r.GetNotifier()
	for {
		select {
		case n := <-notifier:
			switch n.(type) {
			case *consensus.ProposeEvent:
				go r.DoBroadcastProposal(n.(*consensus.ProposeEvent).Proposal)
			case *consensus.VoteEvent:
				go r.DoVote(n.(*consensus.VoteEvent).Vote, r.GetLeader())
			case *consensus.HqcUpdateEvent:
				r.curView = n.(*consensus.HqcUpdateEvent).Qc.ViewNumber
			case *consensus.NewViewEvent:
				r.OnNextSyncView()
			case *consensus.ReceiveNewView:
				view := n.(*consensus.ReceiveNewView)
				if view.ViewNumber > r.curView {
					r.curView = view.ViewNumber
					go r.OnBeat()
				}
			case *consensus.QcFinishEvent:
				if r.GetID() == r.GetLeader() {
					go r.OnBeat()
				}
			case *consensus.DecideEvent:
				go r.doDecide(n.(*consensus.DecideEvent).Cmds)
			}
		}
	}
}

func (r *RoundRobinPM) doDecide(cmds []byte) {
	logger.Info("consensus complete", "cmds", cmds)
}

func (r *RoundRobinPM) OnNextSyncView() {
	leader := r.GetLeader()
	logger.Debug("enter next view", "curView", r.curView, "nextLeader", leader)
	r.curView++
	if leader == r.GetID() {
		go r.OnBeat()
		return
	}
	viewMsg := &pb.Message{Type: &pb.Message_NewView{
		NewView: &pb.NewView{ViewNumber: r.curView, GenericQc: r.GetHighQC()}}}
	_ = r.UnicastMsg(viewMsg, leader)
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
	case <-time.After(time.Second):
	}
}
