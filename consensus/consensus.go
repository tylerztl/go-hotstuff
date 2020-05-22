package consensus

import (
	"context"
	"crypto"

	"github.com/zhigui-projects/go-hotstuff/pacemaker"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

type HotStuffBase struct {
	*HotStuffCore
	pacemaker.PaceMaker
	execFunc func(cmds []byte)
}

func NewHotStuffBase(id ReplicaID, address string, signer crypto.Signer, replicas *ReplicaConf) *HotStuffBase {
	return &HotStuffBase{
		HotStuffCore: NewHotStuffCore(id, address, signer, replicas),
		PaceMaker:    pacemaker.NewRoundRobinPM(),
	}
}

func (hsb *HotStuffBase) HandlePropose(proposal *pb.Proposal) {
	if proposal.Block == nil {
		logger.Warn("handle propose with empty block", "proposer", proposal.Proposer)
		return
	}

	if err := hsb.OnReceiveProposal(proposal.Block); err != nil {
		logger.Warn("handle propose catch error", "error", err)
		return
	}
}

func (hsb *HotStuffBase) HandleVote(vote *pb.Vote) {
	if ok := hsb.replicas.VerifyVote(vote); !ok {
		return
	}
	if err := hsb.OnReceiveVote(vote); err != nil {
		logger.Warn("handle vote catch error", "error", err)
		return
	}
}

func (hsb *HotStuffBase) doBroadcastProposal(proposal *pb.Proposal) {

}

func (hsb *HotStuffBase) doVote(vote *pb.Vote) {

}

func (hsb *HotStuffBase) doDecide(cmds []byte) {
	hsb.execFunc(cmds)
}

func (hsb *HotStuffBase) GetID() ReplicaID {
	return hsb.id
}

func (hsb *HotStuffBase) GetVoteHeight() int64 {
	return hsb.voteHeight
}

func (hsb *HotStuffBase) Start(ctx context.Context) {
	notifier := hsb.GetNotifier()
	for {
		select {
		case n := <-notifier:
			n.Execute(hsb)
		case <-ctx.Done():
			return
		}
	}
}
