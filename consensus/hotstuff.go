package consensus

import (
	"bytes"
	"encoding/hex"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
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
	// height of the block last voted for
	voteHeight int64

	mut *sync.Mutex

	blockCache *sync.Map

	voteSet map[int64][]*pb.Vote

	// identity of the replica itself
	id ReplicaID

	signer Signer

	replicas *ReplicaConf

	notifyChan chan EventNotifier
}

func NewHotStuffCore(id ReplicaID, signer Signer, replicas *ReplicaConf) *HotStuffCore {
	// TODO: node restart, sync block
	genesis := &pb.Block{
		Height: 0,
	}
	hash := GetBlockHash(genesis)
	hsc := &HotStuffCore{
		genesisBlock: genesis,
		voteHeight:   0,
		mut:          &sync.Mutex{},
		blockCache:   &sync.Map{},
		voteSet:      make(map[int64][]*pb.Vote),
		id:           id,
		signer:       signer,
		replicas:     replicas,
		notifyChan:   make(chan EventNotifier),
	}
	genesis.SelfQc = &pb.QuorumCert{ViewNumber: -1, BlockHash: hash, Signs: make(map[int64]*pb.PartCert)}
	hsc.genericQC = genesis.SelfQc
	hsc.lockedQC = genesis.SelfQc
	hsc.execQC = genesis.SelfQc
	hsc.blockCache.Store(hex.EncodeToString(hash), genesis)
	return hsc
}

func (hsc *HotStuffCore) OnPropose(curView int64, parentHash, cmds []byte) error {
	v, ok := hsc.blockCache.Load(hex.EncodeToString(parentHash))
	if !ok {
		return errors.Errorf("parent block [%s] not found", hex.EncodeToString(parentHash))
	}
	block := v.(*pb.Block)
	// create the new block
	newBlock := hsc.createLeaf(parentHash, cmds, block.Height+1)
	// add new block to storage
	hash := GetBlockHash(newBlock)
	hsc.blockCache.Store(hex.EncodeToString(hash), newBlock)
	logger.Debug("create leaf node", "view", curView, "height", newBlock.Height, "cmds", string(cmds), "hash", hex.EncodeToString(hash))
	// create quorum cert
	newBlock.SelfQc = &pb.QuorumCert{ViewNumber: curView, BlockHash: hash, Signs: make(map[int64]*pb.PartCert)}
	err := hsc.update(newBlock)
	if err != nil {
		return err
	}
	// self vote
	// TODO 小于是可能存在fork branch
	if newBlock.Height < hsc.voteHeight {
		return errors.Errorf("new block should be higher than vote height, newHeight:%d, voteHeight:%d", newBlock.Height, hsc.voteHeight)
	}
	hsc.voteHeight = newBlock.Height
	vote, err := hsc.voteProposal(curView, hash)
	if err != nil {
		return err
	}
	if err = hsc.OnReceiveVote(vote); err != nil {
		return err
	}

	logger.Debug("proposed new proposal", "view", curView, "height", newBlock.Height, "cmds", string(cmds), "parentHash", hex.EncodeToString(parentHash))
	// broadcast proposal to other replicas
	hsc.notify(&ProposeEvent{&pb.Proposal{ViewNumber: curView, Proposer: int64(hsc.id), Block: newBlock}})
	return nil
}

func (hsc *HotStuffCore) OnReceiveProposal(prop *pb.Proposal) error {
	block := prop.Block
	hsc.blockCache.Store(hex.EncodeToString(block.SelfQc.BlockHash), block)

	if err := hsc.update(block); err != nil {
		return err
	}

	hsc.mut.Lock()
	if block.Height > hsc.voteHeight {
		jb, err := hsc.getJustifyBlock(block)
		if err != nil {
			hsc.mut.Unlock()
			return err
		}
		lockedb, err := hsc.getBlockByHash(hsc.lockedQC.BlockHash)
		if err != nil {
			hsc.mut.Unlock()
			return err
		}
		if jb.Height > lockedb.Height {
			// liveness condition
			hsc.voteHeight = block.Height
		} else {
			// safety condition (extend the locked branch)
			nblk := block
			for i := block.Height - lockedb.Height; i > 0; i-- {
				if nblk, err = hsc.getBlockByHash(nblk.ParentHash); err != nil {
					hsc.mut.Unlock()
					return err
				}
			}
			if bytes.Equal(nblk.SelfQc.BlockHash, lockedb.SelfQc.BlockHash) {
				hsc.voteHeight = block.Height
			}
		}
	}
	hsc.mut.Unlock()

	vote, err := hsc.voteProposal(prop.ViewNumber, block.SelfQc.BlockHash)
	if err != nil {
		return err
	}

	// send voteMsg to nextView leader
	hsc.notify(&ReceiveProposalEvent{prop, vote})

	return nil
}

func (hsc *HotStuffCore) OnReceiveVote(vote *pb.Vote) error {
	logger.Debug("enter receive vote", "vote", vote)
	block, _ := hsc.getBlockByHash(vote.BlockHash)
	if block == nil {
		// proposal广播消息在vote消息之后到达
		hsc.addVoteMsg(vote)
		return nil
	}

	hsc.mut.Lock()
	if len(block.SelfQc.Signs) >= utils.GetQuorumSize(hsc.replicas.Metadata) {
		logger.Debug("receive vote number already satisfied quorum size", "view", vote.ViewNumber)
		hsc.mut.Unlock()
		return nil
	}

	if _, ok := block.SelfQc.Signs[vote.Voter]; ok {
		hsc.mut.Unlock()
		return errors.Errorf("duplicate vote for %s from %d", hex.EncodeToString(vote.BlockHash), vote.Voter)
	}

	block.SelfQc.Signs[vote.Voter] = vote.Cert
	hsc.mut.Unlock()

	hsc.applyVotes(vote.ViewNumber, block)

	if len(block.SelfQc.Signs) < utils.GetQuorumSize(hsc.replicas.Metadata) {
		return nil
	}

	logger.Debug("receive vote number already satisfied quorum size", "view", vote.ViewNumber)
	hsc.UpdateHighestQC(block, block.SelfQc)
	hsc.notify(&QcFinishEvent{Proposer: block.Proposer, Qc: block.SelfQc})

	return nil
}

func (hsc *HotStuffCore) voteProposal(viewNumber int64, hash []byte) (*pb.Vote, error) {
	logger.Debug("enter vote proposal", "hash", hex.EncodeToString(hash))
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

	if !bytes.Equal(block2.ParentHash, block2.Justify.BlockHash) || !bytes.Equal(block1.ParentHash, block1.Justify.BlockHash) {
		logger.Warn("decide phase failed, not build three-chain", "block2", block2.Height,
			"block1", block1.Height, "block0", block0.Height)
		return nil
	}

	hsc.mut.Lock()
	var execHeight int64
	if hsc.execQC != nil {
		execBlock, err := hsc.getBlockByHash(hsc.execQC.BlockHash)
		if err != nil {
			hsc.mut.Unlock()
			return err
		}
		execHeight = execBlock.Height
	}
	hsc.execQC = block1.Justify
	hsc.mut.Unlock()

	blockHash := GetBlockHash(block0)
	for i := block0.Height - execHeight; i > 0; i-- {
		nblk, err := hsc.getBlockByHash(blockHash)
		if err != nil {
			return err
		}
		logger.Debug("DECIDE phase, do consensus", "blockHeight", nblk.Height)
		hsc.notify(&DecideEvent{nblk})
		//hsc.blockCache.Delete(hex.EncodeToString(blockHash))
		blockHash = nblk.ParentHash
	}

	// TODO delete decide block
	// TODO: also commit the uncles/aunts

	return nil
}

func (hsc *HotStuffCore) UpdateHighestQC(block *pb.Block, qc *pb.QuorumCert) {
	logger.Debug("enter update highest qc", "blockHeight", block.Height)
	if !hsc.replicas.VerifyQuorumCert(qc) {
		logger.Debug("QC not verified", "blockHeight", block.Height, "voteNumber", len(qc.Signs))
		return
	}

	hsc.mut.Lock()
	defer hsc.mut.Unlock()

	if hsc.genericQC == nil {
		hsc.genericQC = qc
		return
	}
	highBlock, err := hsc.getBlockByHash(hsc.genericQC.BlockHash)
	if err != nil {
		logger.Error("updateHighestQC call getBlockByHash failed", "error", err)
		return
	}
	logger.Debug("update highest QC", "newHeight", block.Height, "oldHeight", highBlock.Height)
	if block.Height > highBlock.Height {
		hsc.genericQC = qc
		hsc.notify(&HqcUpdateEvent{qc})
	}
}

func (hsc *HotStuffCore) updateLockedQC(block *pb.Block, qc *pb.QuorumCert) {
	hsc.mut.Lock()
	defer hsc.mut.Unlock()

	if hsc.lockedQC == nil {
		logger.Debug("COMMIT phase, update locked QC", "blockHeight", block.Height, "qc", qc)
		hsc.lockedQC = qc
	}
	lockedBlock, err := hsc.getBlockByHash(hsc.lockedQC.BlockHash)
	if err != nil {
		logger.Error("updateLockedQC call getBlockByHash failed", "error", err)
		return
	}

	if block.Height > lockedBlock.Height {
		logger.Debug("COMMIT phase, update locked QC", "blockHeight", block.Height, "qc", qc)
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
	sig, err := hsc.signer.Sign(hash)
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
		logger.Warn("block's justify is null", "blockHeight", block.Height)
		return nil, errors.New("block's justify is null")
	}

	justifyBlock, ok := hsc.blockCache.Load(hex.EncodeToString(block.Justify.BlockHash))
	if !ok {
		logger.Warn("justify block not found from cache", "blockHeight", block.Height)
		return nil, errors.New("justify block not found")
	}
	return justifyBlock.(*pb.Block), nil
}

func (hsc *HotStuffCore) getBlockByHash(hash []byte) (*pb.Block, error) {
	block, ok := hsc.blockCache.Load(hex.EncodeToString(hash))
	if !ok {
		logger.Warn("block not found from cache", "blockhash", hex.EncodeToString(hash))
		return nil, errors.Errorf("block not found with hash: %s", hex.EncodeToString(hash))
	}
	return block.(*pb.Block), nil
}

func (hsc *HotStuffCore) addVoteMsg(vote *pb.Vote) {
	hsc.mut.Lock()
	defer hsc.mut.Unlock()

	voteSlice, ok := hsc.voteSet[vote.ViewNumber]
	if ok {
		hsc.voteSet[vote.ViewNumber] = append(voteSlice, vote)
	} else {
		hsc.voteSet[vote.ViewNumber] = []*pb.Vote{vote}
	}
}

func (hsc *HotStuffCore) applyVotes(viewNumber int64, block *pb.Block) {
	hsc.mut.Lock()
	defer hsc.mut.Unlock()

	votes, ok := hsc.voteSet[viewNumber]
	if !ok {
		return
	}

	for _, v := range votes {
		if _, ok := block.SelfQc.Signs[v.Voter]; ok {
			continue
		}
		block.SelfQc.Signs[v.Voter] = v.Cert
	}

	delete(hsc.voteSet, viewNumber)
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
	hsc.mut.Lock()
	defer hsc.mut.Unlock()
	return hsc.voteHeight
}

func (hsc *HotStuffCore) GetHighQC() *pb.QuorumCert {
	hsc.mut.Lock()
	defer hsc.mut.Unlock()
	return hsc.genericQC
}

func (hsc *HotStuffCore) GetReplicas() *ReplicaConf {
	return hsc.replicas
}
