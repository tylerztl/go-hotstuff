package consensus

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/pkg/errors"
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

	// identity of the replica itself
	id ReplicaID

	crypto.Signer

	replicas *ReplicaConf

	notifyChan chan EventNotifier
}

func NewHotStuffCore(id ReplicaID, signer crypto.Signer, replicas *ReplicaConf) *HotStuffCore {
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
		id:           id,
		Signer:       signer,
		replicas:     replicas,
		notifyChan:   make(chan EventNotifier, 10),
	}
	genesis.SelfQc = &pb.QuorumCert{BlockHash: hash, Signs: make(map[int64]*pb.PartCert)}
	hsc.genericQC = genesis.SelfQc
	hsc.lockedQC = genesis.SelfQc
	hsc.execQC = genesis.SelfQc
	hsc.blockCache.Store(hex.EncodeToString(hash), genesis)
	return hsc
}

func (hsc *HotStuffCore) OnPropose(parentHash, cmds []byte) error {
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
	// create quorum cert
	newBlock.SelfQc = &pb.QuorumCert{BlockHash: hash, Signs: make(map[int64]*pb.PartCert)}
	err := hsc.update(newBlock)
	if err != nil {
		return err
	}
	// self vote
	if newBlock.Height <= hsc.voteHeight {
		return errors.New("new block should be higher than vote height")
	}
	hsc.voteHeight = newBlock.Height
	vote, err := hsc.voteProposal(hash, false)
	if err != nil {
		return err
	}
	if err = hsc.OnReceiveVote(vote); err != nil {
		return err
	}

	// broadcast proposal to other replicas
	hsc.notify(&proposeEvent{proposal: &pb.Proposal{Proposer: int64(hsc.id), Block: block}})
	return nil
}

func (hsc *HotStuffCore) OnReceiveProposal(block *pb.Block) error {
	if err := hsc.update(block); err != nil {
		return err
	}

	{
		hsc.mut.Lock()
		defer hsc.mut.Unlock()

		if block.Height > hsc.voteHeight {
			jb, err := hsc.getJustifyBlock(block)
			if err != nil {
				return err
			}
			lockedb, err := hsc.getBlockByHash(hsc.lockedQC.BlockHash)
			if err != nil {
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
						return err
					}
				}
				if bytes.Equal(nblk.SelfQc.BlockHash, lockedb.SelfQc.BlockHash) {
					hsc.voteHeight = block.Height
				}
			}
		}
	}

	// TODO: qc finish ?

	hsc.notify(&receiveProposalEvent{&pb.Proposal{Proposer: int64(hsc.id), Block: block}})

	if _, err := hsc.voteProposal(block.SelfQc.BlockHash, true); err != nil {
		return err
	}

	return nil
}

func (hsc *HotStuffCore) OnReceiveVote(vote *pb.Vote) error {
	block, err := hsc.getBlockByHash(vote.BlockHash)
	if err != nil {
		return err
	}

	hsc.mut.Lock()
	if len(block.SelfQc.Signs) >= hsc.replicas.QuorumSize {
		return nil
	}

	if _, ok := block.SelfQc.Signs[vote.Voter]; ok {
		return errors.Errorf("duplicate vote for %s from %d", hex.EncodeToString(vote.BlockHash), vote.Voter)
	}

	block.SelfQc.Signs[vote.Voter] = vote.Cert

	if len(block.SelfQc.Signs) < hsc.replicas.QuorumSize {
		return nil
	}
	hsc.mut.Unlock()

	hsc.updateHighestQC(block, block.SelfQc)
	hsc.notify(&qcFinishEvent{block.SelfQc})

	return nil
}

func (hsc *HotStuffCore) voteProposal(hash []byte, deliver bool) (*pb.Vote, error) {
	cert, err := hsc.createPartCert(hash)
	if err != nil {
		return nil, err
	}
	vote := &pb.Vote{
		Voter:     int64(hsc.id),
		BlockHash: hash,
		Cert:      cert,
	}
	if deliver {
		// send voteMsg to nextView leader
		hsc.notify(&voteEvent{vote})
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
	hsc.updateHighestQC(block2, block.Justify)

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
			return err
		}
		execHeight = execBlock.Height
	}
	hsc.execQC = block1.Justify
	hsc.mut.Unlock()

	for i, nblk := block0.Height-execHeight, block0; i > 0; i-- {
		hsc.blockCache.Delete(hex.EncodeToString(GetBlockHash(nblk)))

		hsc.notify(&decideEvent{nblk.Cmds})

		if nblk, err = hsc.getBlockByHash(nblk.ParentHash); err != nil {
			return err
		}
	}

	// TODO: also commit the uncles/aunts

	return nil
}

func (hsc *HotStuffCore) updateHighestQC(block *pb.Block, qc *pb.QuorumCert) {
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

	if block.Height > highBlock.Height {
		hsc.genericQC = qc

		hsc.notify(&hqcUpdateEvent{qc})
	}
}

func (hsc *HotStuffCore) updateLockedQC(block *pb.Block, qc *pb.QuorumCert) {
	hsc.mut.Lock()
	defer hsc.mut.Unlock()

	if hsc.lockedQC == nil {
		hsc.lockedQC = qc
	}
	lockedBlock, err := hsc.getBlockByHash(hsc.genericQC.BlockHash)
	if err != nil {
		logger.Error("updateLockedQC call getBlockByHash failed", "error", err)
		return
	}

	if block.Height > lockedBlock.Height {
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
		Justify:    hsc.genericQC,
	}
}

func (hsc *HotStuffCore) createPartCert(hash []byte) (*pb.PartCert, error) {
	sig, err := hsc.Signer.Sign(rand.Reader, hash, nil)
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

func (hsc *HotStuffCore) GetNotifier() <-chan EventNotifier {
	return hsc.notifyChan
}

func (hsc *HotStuffCore) notify(n EventNotifier) {
	hsc.notifyChan <- n
}
