package consensus

import (
	"context"
	"crypto"
)

type Consensus interface {
}

type HotStuffBase struct {
	hsc *HotStuffCore
}

func NewHotStuffBase(id ReplicaID, address string, signer crypto.Signer, replicas *ReplicaConf) *HotStuffBase {
	return &HotStuffBase{
		hsc: NewHotStuffCore(id, address, signer, replicas),
	}
}

func (hsb *HotStuffBase) HandlePropose() {

}

func (hsb *HotStuffBase) HandleVote() {

}

func (hsb *HotStuffBase) Run(ctx context.Context) {
	notifier := hsb.hsc.GetNotifier()
	for {
		select {
		case n := <-notifier:
			n.Execute(hsb)
		case <-ctx.Done():
			return
		}
	}
}
