/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package consensus

import (
	"github.com/zhigui-projects/go-hotstuff/api"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

type EventNotifier interface {
	ExecuteEvent(pm api.PaceMaker)
}

type ProposeEvent struct {
	Proposal *pb.Proposal
}

func (p *ProposeEvent) ExecuteEvent(pm api.PaceMaker) {
	pm.OnProposeEvent(p.Proposal)
}

type ReceiveProposalEvent struct {
	Proposal *pb.Proposal
	Vote     *pb.Vote
}

func (r *ReceiveProposalEvent) ExecuteEvent(pm api.PaceMaker) {
	pm.OnReceiveProposal(r.Proposal, r.Vote)
}

type ReceiveNewViewEvent struct {
	ReplicaId int64
	View      *pb.NewView
}

func (r *ReceiveNewViewEvent) ExecuteEvent(pm api.PaceMaker) {
	pm.OnReceiveNewView(r.ReplicaId, r.View)
}

type QcFinishEvent struct {
	Proposer int64
	Qc       *pb.QuorumCert
}

func (q *QcFinishEvent) ExecuteEvent(pm api.PaceMaker) {
	pm.OnQcFinishEvent(q.Qc)
}

type HqcUpdateEvent struct {
	Qc *pb.QuorumCert
}

func (h *HqcUpdateEvent) ExecuteEvent(pm api.PaceMaker) {
	pm.UpdateQcHigh(h.Qc.ViewNumber, h.Qc)
}

type DecideEvent struct {
	Block *pb.Block
}

func (d *DecideEvent) ExecuteEvent(pm api.PaceMaker) {
	pm.DoDecide(d.Block)
}

type MsgExecutor interface {
	ExecuteMessage(base *HotStuffBase)
}

type msgEvent struct {
	src ReplicaID
	msg *pb.Message
}

func (m *msgEvent) ExecuteMessage(base *HotStuffBase) {
	base.handleMessage(m.src, m.msg)
}
