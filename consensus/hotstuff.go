/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package consensus

import (
	"bytes"
	"encoding/hex"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/zhigui-projects/go-hotstuff/api"
	"github.com/zhigui-projects/go-hotstuff/common/db/memorydb"
	"github.com/zhigui-projects/go-hotstuff/common/utils"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

type HotStuffCore struct {
	// the genesis block
	genesisBlock *pb.Block
	/*
		b = b'.justify.node
		b' = b''.justify.node
		b'' = b∗.justify.node
	*/
	// highest QC, b*.justify
	genericQC *pb.QuorumCert
	// locked block, b''.justify
	lockedQC *pb.QuorumCert
	// last executed block, b'.justify
	execQC *pb.QuorumCert

	qcLock sync.Mutex

	// height of the block last voted for
	voteHeight int64

	vLock sync.RWMutex

	mut sync.Mutex

	blockCache api.Database

	// vote msg cache queue
	voteSet map[string][]*pb.Vote

	// identity of the replica itself
	id ReplicaID

	api.Signer

	replicas *ReplicaConf

	notifyChan chan EventNotifier

	logger api.Logger
}

func NewHotStuffCore(id ReplicaID, signer api.Signer, replicas *ReplicaConf, logger api.Logger) *HotStuffCore {
	// TODO: node restart, sync block
	genesis := &pb.Block{
		Height: 0,
	}
	hash := GetBlockHash(genesis)
	hsc := &HotStuffCore{
		genesisBlock: genesis,
		voteHeight:   0,
		blockCache:   memorydb.NewLRUCache(30),
		voteSet:      make(map[string][]*pb.Vote),
		id:           id,
		Signer:       signer,
		replicas:     replicas,
		notifyChan:   make(chan EventNotifier),
		logger:       logger,
	}
	genesis.SelfQc = &pb.QuorumCert{ViewNumber: -1, BlockHash: hash, Signs: make(map[int64]*pb.PartCert)}
	hsc.genericQC = genesis.SelfQc
	hsc.lockedQC = genesis.SelfQc
	hsc.execQC = genesis.SelfQc

	hsc.StoreBlock(genesis)
	return hsc
}

func (hsc *HotStuffCore) OnPropose(curView int64, parentHash, cmds []byte) error {
	block, err := hsc.LoadBlock(parentHash)
	if err != nil {
		return errors.Errorf("parent block not found, err: %v", err)
	}
	// create the new block
	newBlock := hsc.createLeaf(parentHash, cmds, block.Height+1)
	// 有可能存在fork branch，分叉是因为proposal信息在view-change信息之后到达
	if newBlock.Height <= hsc.GetVoteHeight() {
		return errors.Errorf("new block should be higher than vote height, newHeight:%d, voteHeight:%d",
			newBlock.Height, hsc.GetVoteHeight())
	}
	// add new block to storage
	hash := GetBlockHash(newBlock)
	hsc.logger.Info("create leaf node", "view", curView, "blockHeight", newBlock.Height,
		"hash", hex.EncodeToString(hash), "parentHash", hex.EncodeToString(parentHash))
	// create quorum cert
	newBlock.SelfQc = &pb.QuorumCert{ViewNumber: curView, BlockHash: hash, Signs: make(map[int64]*pb.PartCert)}
	hsc.StoreBlock(newBlock)
	err = hsc.update(newBlock)
	if err != nil {
		return err
	}
	// self vote
	hsc.setVoteHeight(newBlock.Height)
	vote, err := hsc.voteProposal(curView, newBlock.Height, hash)
	if err != nil {
		return err
	}
	if err = hsc.OnProposalVote(vote); err != nil {
		return err
	}

	hsc.logger.Debug("proposed new proposal", "view", curView, "height", newBlock.Height, "cmds", string(cmds))
	// broadcast proposal to other replicas
	hsc.notify(&ProposeEvent{&pb.Proposal{ViewNumber: curView, Proposer: int64(hsc.id), Block: newBlock}})
	return nil
}

func (hsc *HotStuffCore) OnReceiveProposal(prop *pb.Proposal) error {
	hsc.logger.Info("receive proposal msg", "proposer", prop.Block.Proposer, "height", prop.Block.Height,
		"voteHeight", hsc.GetVoteHeight(), "hash", hex.EncodeToString(prop.Block.SelfQc.BlockHash))

	block := prop.Block
	hsc.StoreBlock(block)

	if err := hsc.update(block); err != nil {
		return err
	}

	if block.Height > hsc.GetVoteHeight() {
		jb, err := hsc.getJustifyBlock(block)
		if err != nil {
			return err
		}
		lockedb, err := hsc.LoadBlock(hsc.lockedQC.BlockHash)
		if err != nil {
			return err
		}
		if jb.Height > lockedb.Height {
			// liveness condition
			hsc.setVoteHeight(block.Height)
		} else {
			// safety condition (extend the locked branch)
			nblk := block
			for i := block.Height - lockedb.Height; i > 0; i-- {
				if nblk, err = hsc.LoadBlock(nblk.ParentHash); err != nil {
					return err
				}
			}
			if bytes.Equal(nblk.SelfQc.BlockHash, lockedb.SelfQc.BlockHash) {
				hsc.setVoteHeight(block.Height)
			}
		}
	}

	vote, err := hsc.voteProposal(prop.ViewNumber, block.Height, block.SelfQc.BlockHash)
	if err != nil {
		return err
	}

	// send voteMsg to nextView leader
	hsc.notify(&ReceiveProposalEvent{prop, vote})

	return nil
}

func (hsc *HotStuffCore) OnProposalVote(vote *pb.Vote) error {
	block, _ := hsc.LoadBlock(vote.BlockHash)
	if block == nil {
		// Proposal Message可能在Vote Message之后到达
		hsc.addVoteMsg(vote)
		return nil
	}

	hsc.mut.Lock()
	if len(block.SelfQc.Signs) >= utils.GetQuorumSize(hsc.replicas.Metadata) {
		hsc.logger.Info("receive vote number already satisfied quorum size", "view", vote.ViewNumber)
		hsc.mut.Unlock()
		return nil
	}

	if _, ok := block.SelfQc.Signs[vote.Voter]; ok {
		hsc.mut.Unlock()
		return errors.Errorf("duplicate vote for %s from %d", hex.EncodeToString(vote.BlockHash), vote.Voter)
	}

	if block.SelfQc.Signs == nil {
		block.SelfQc.Signs = map[int64]*pb.PartCert{}
	}
	block.SelfQc.Signs[vote.Voter] = vote.Cert

	hsc.applyVotes(block)
	if len(block.SelfQc.Signs) < utils.GetQuorumSize(hsc.replicas.Metadata) {
		hsc.mut.Unlock()
		return nil
	}
	hsc.mut.Unlock()

	hsc.logger.Info("received vote msg number already satisfied quorum size", "blockHeight", block.Height,
		"blockHash", hex.EncodeToString(vote.BlockHash))
	hsc.UpdateHighestQC(block, block.SelfQc)
	hsc.notify(&QcFinishEvent{Proposer: block.Proposer, Qc: block.SelfQc})

	return nil
}

func (hsc *HotStuffCore) OnNewView(signer int64, view *pb.NewView) error {
	hsc.notify(&ReceiveNewViewEvent{signer, view})
	return nil
}

func (hsc *HotStuffCore) voteProposal(viewNumber, height int64, hash []byte) (*pb.Vote, error) {
	hsc.logger.Info("vote proposal", "voter", hsc.id, "view", viewNumber,
		"blockHeight", height, "hash", hex.EncodeToString(hash))
	cert, err := hsc.createPartCert(hash)
	if err != nil {
		return nil, err
	}
	vote := &pb.Vote{
		ViewNumber: viewNumber,
		Voter:      int64(hsc.id),
		BlockHash:  hash,
		Cert:       cert,
	}

	return vote, nil
}

func (hsc *HotStuffCore) update(block *pb.Block) error {
	// three-chain judge
	// block = b*, block2 = b'', block1 = b', block0 = b

	// PRE-COMMIT phase on b''
	block2, err := hsc.getJustifyBlock(block)
	if err != nil {
		return err
	}
	hsc.UpdateHighestQC(block2, block.Justify)

	// COMMIT phase on b'
	block1, err := hsc.getJustifyBlock(block2)
	if err != nil {
		return nil
	}
	hsc.updateLockedQC(block1, block2.Justify)

	// DECIDE phase on b
	block0, err := hsc.getJustifyBlock(block1)
	if err != nil {
		return nil
	}

	if !bytes.Equal(block2.ParentHash, block2.Justify.BlockHash) ||
		!bytes.Equal(block1.ParentHash, block1.Justify.BlockHash) {
		hsc.logger.Warning("decide phase failed, not build three-chain", "block2", block2.Height,
			"block1", block1.Height, "block0", block0.Height)
		return nil
	}

	hsc.qcLock.Lock()
	var execHeight int64
	if hsc.execQC != nil {
		execBlock, err := hsc.LoadBlock(hsc.execQC.BlockHash)
		if err != nil {
			hsc.qcLock.Unlock()
			return err
		}
		execHeight = execBlock.Height
	}
	hsc.execQC = block1.Justify
	hsc.qcLock.Unlock()

	blockHash := GetBlockHash(block0)
	for i := block0.Height - execHeight; i > 0; i-- {
		nblk, err := hsc.LoadBlock(blockHash)
		if err != nil {
			return err
		}
		hsc.logger.Info("DECIDE phase, do consensus", "blockHeight", nblk.Height)
		hsc.notify(&DecideEvent{nblk})
		blockHash = nblk.ParentHash
	}

	// TODO: also commit the uncles/aunts

	return nil
}

func (hsc *HotStuffCore) UpdateHighestQC(block *pb.Block, qc *pb.QuorumCert) {
	hsc.logger.Debug("enter update highest qc", "blockHeight", block.Height, "qc", qc)
	if !hsc.replicas.VerifyQuorumCert(qc) {
		hsc.logger.Debug("QC not verified", "blockHeight", block.Height, "voteNumber", len(qc.Signs))
		return
	}

	hsc.qcLock.Lock()
	defer hsc.qcLock.Unlock()

	if hsc.genericQC == nil {
		hsc.genericQC = qc
		return
	}
	highBlock, err := hsc.LoadBlock(hsc.genericQC.BlockHash)
	if err != nil {
		hsc.logger.Error("updateHighestQC call getBlockByHash failed", "error", err)
		return
	}
	hsc.logger.Debug("update highest QC", "newHeight", block.Height, "oldHeight", highBlock.Height)
	if block.Height > highBlock.Height {
		hsc.logger.Info("PRE-COMMIT phase, update highQC", "newHeight", block.Height, "oldHeight", highBlock.Height)
		hsc.genericQC = qc
		hsc.notify(&HqcUpdateEvent{qc})
	}
}

func (hsc *HotStuffCore) updateLockedQC(block *pb.Block, qc *pb.QuorumCert) {
	hsc.qcLock.Lock()
	defer hsc.qcLock.Unlock()

	if hsc.lockedQC == nil {
		hsc.logger.Info("COMMIT phase, update locked QC", "view", qc.ViewNumber, "blockHeight",
			block.Height, "hash", hex.EncodeToString(qc.BlockHash))
		hsc.lockedQC = qc
	}
	lockedBlock, err := hsc.LoadBlock(hsc.lockedQC.BlockHash)
	if err != nil {
		hsc.logger.Error("updateLockedQC call getBlockByHash failed", "error", err)
		return
	}

	if block.Height > lockedBlock.Height {
		hsc.logger.Info("COMMIT phase, update locked QC", "view", qc.ViewNumber, "blockHeight",
			block.Height, "hash", hex.EncodeToString(qc.BlockHash))
		hsc.lockedQC = qc
	}
}

func (hsc *HotStuffCore) createLeaf(parentHash, cmds []byte, height int64) *pb.Block {
	return &pb.Block{
		ParentHash: parentHash,
		Cmds:       cmds,
		Proposer:   int64(hsc.id),
		Height:     height,
		Timestamp:  time.Now().UnixNano(),
		Justify:    proto.Clone(hsc.genericQC).(*pb.QuorumCert),
	}
}

func (hsc *HotStuffCore) createPartCert(hash []byte) (*pb.PartCert, error) {
	sig, err := hsc.Sign(hash)
	if err != nil {
		return nil, err
	}
	return &pb.PartCert{ReplicaId: int64(hsc.id), Signature: sig}, nil
}

func (hsc *HotStuffCore) getJustifyBlock(block *pb.Block) (*pb.Block, error) {
	if block == nil {
		return nil, errors.New("empty block")
	}
	if block.Justify == nil {
		hsc.logger.Warning("block's justify is null", "blockHeight", block.Height)
		return nil, errors.New("block's justify is null")
	}

	justifyBlock, err := hsc.LoadBlock(block.Justify.BlockHash)
	if err != nil {
		hsc.logger.Warning("justify block not found from db", "blockHeight", block.Height, "err", err)
		return nil, err
	}
	return justifyBlock, nil
}

func (hsc *HotStuffCore) addVoteMsg(vote *pb.Vote) {
	hsc.mut.Lock()
	defer hsc.mut.Unlock()

	blockHash := hex.EncodeToString(vote.BlockHash)
	voteSlice, ok := hsc.voteSet[blockHash]
	if ok {
		hsc.voteSet[blockHash] = append(voteSlice, vote)
	} else {
		hsc.voteSet[blockHash] = []*pb.Vote{vote}
	}
}

func (hsc *HotStuffCore) applyVotes(block *pb.Block) {
	blockHash := hex.EncodeToString(block.SelfQc.BlockHash)
	if votes, ok := hsc.voteSet[blockHash]; ok {
		for _, v := range votes {
			if _, ok := block.SelfQc.Signs[v.Voter]; ok {
				continue
			}
			block.SelfQc.Signs[v.Voter] = v.Cert
		}
		delete(hsc.voteSet, blockHash)
	}
}

func (hsc *HotStuffCore) notify(n EventNotifier) {
	hsc.notifyChan <- n
}

func (hsc *HotStuffCore) GetNotifier() <-chan EventNotifier {
	return hsc.notifyChan
}

func (hsc *HotStuffCore) GetID() int64 {
	return int64(hsc.id)
}

func (hsc *HotStuffCore) GetVoteHeight() int64 {
	hsc.vLock.RLock()
	defer hsc.vLock.RUnlock()
	return hsc.voteHeight
}

func (hsc *HotStuffCore) setVoteHeight(vHeight int64) {
	hsc.vLock.Lock()
	defer hsc.vLock.Unlock()
	hsc.voteHeight = vHeight
}

func (hsc *HotStuffCore) GetHighQC() *pb.QuorumCert {
	hsc.qcLock.Lock()
	defer hsc.qcLock.Unlock()
	return hsc.genericQC
}

func (hsc *HotStuffCore) GetReplicas() *ReplicaConf {
	return hsc.replicas
}

func (hsc *HotStuffCore) StoreBlock(block *pb.Block) {
	blockHash := hex.EncodeToString(block.SelfQc.BlockHash)
	if err := hsc.blockCache.Put(blockHash, block); err != nil {
		hsc.logger.Error("store block data from db failed", "err", err, "hash", blockHash)
	}
	hsc.logger.Info("stored block to db", "hash", blockHash)
}

func (hsc *HotStuffCore) LoadBlock(hash []byte) (*pb.Block, error) {
	blockHash := hex.EncodeToString(hash)
	block, err := hsc.blockCache.Get(blockHash)
	if err != nil {
		return nil, err
	}
	return block.(*pb.Block), nil
}
