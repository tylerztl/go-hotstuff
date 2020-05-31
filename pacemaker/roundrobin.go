package pacemaker

import (
	"context"

	"github.com/pkg/errors"
	"github.com/zhigui-projects/go-hotstuff/consensus"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

type RoundRobinPM struct {
	*consensus.HotStuffBase
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

	for {
		select {
		case n := <-r.GetNotifier():
			switch n.(type) {
			case *consensus.ProposeEvent:
				r.DoBroadcastProposal(n.(*consensus.ProposeEvent).Proposal)
				if r.GetID() == r.GetLeader() {

				}
			case *consensus.ReceiveProposalEvent:
				// TODO
			case *consensus.VoteEvent:
				r.DoVote(n.(*consensus.VoteEvent).Vote, r.GetLeader())
			case *consensus.HqcUpdateEvent:
				// TODO
			case *consensus.ReceiveNewView:
				r.OnBeat()
			case *consensus.QcFinishEvent:
				qc := n.(*consensus.QcFinishEvent)
				// was leader for previous view, but not the leader for next view do view change
				if r.GetID() == qc.Proposer {
					if r.GetID() != r.GetLeader() {
						go r.OnNextSyncView(r.GetLeader())
					} else {
						r.OnBeat()
					}
				}
			case *consensus.DecideEvent:
				r.doDecide(n.(*consensus.DecideEvent).Cmds)
			}
		}
	}
}

func (r *RoundRobinPM) doDecide(cmds []byte) {
	logger.Info("consensus complete", "cmds", cmds)
}

func (r *RoundRobinPM) OnBeat() {
	select {
	case s := <-r.submitC:
		if err := r.OnPropose(r.GetHighQC().BlockHash, s); err != nil {
			logger.Error("propose catch error", "error", err)
		}
	}
}

func (r *RoundRobinPM) OnNextSyncView(leader int64) {
	logger.Debug("enter next view", "leader", leader)
	if leader == r.GetID() {
		r.OnBeat()
		return
	}
	viewMsg := &pb.Message{Type: &pb.Message_NewView{
		NewView: &pb.NewView{GenericQc: r.GetHighQC()}}}
	_ = r.UnicastMsg(viewMsg, leader)
}

func (r *RoundRobinPM) GetLeader() int64 {
	return (r.GetID() + 1) % int64(len(r.Nodes))
}

func (r *RoundRobinPM) Submit(ctx context.Context, req *pb.SubmitRequest) (*pb.SubmitResponse, error) {
	logger.Debug("receive new submit request", "cmds", string(req.Cmds))
	if len(req.Cmds) == 0 {
		return &pb.SubmitResponse{Status: pb.Status_BAD_REQUEST}, errors.New("request data is empty")
	}

	r.submitC <- req.Cmds

	return &pb.SubmitResponse{Status: pb.Status_SUCCESS}, nil
}
