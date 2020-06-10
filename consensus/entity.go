package consensus

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"sync/atomic"

	"github.com/zhigui-projects/go-hotstuff/common/log"
	"github.com/zhigui-projects/go-hotstuff/common/utils"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

var logger = log.GetLogger("module", "consensus")

type ReplicaID int64

type Verifier interface {
	Verify(signature, digest []byte) (bool, error)
}

type Signer interface {
	Sign(digest []byte) ([]byte, error)
}

// ReplicaInfo holds information about a replica
type ReplicaInfo struct {
	Verifier Verifier
}

type ReplicaConf struct {
	Metadata *pb.ConfigMetadata
	Replicas map[ReplicaID]*ReplicaInfo
}

// VerifyQuorumCert will verify a QuorumCert from public keys stored in ReplicaConf
func (rc *ReplicaConf) VerifyQuorumCert(qc *pb.QuorumCert) bool {
	if qc == nil || len(qc.Signs) < utils.GetQuorumSize(rc.Metadata) {
		return false
	}
	var wg sync.WaitGroup
	var numVerified uint64 = 0
	for id, pc := range qc.Signs {
		info, ok := rc.Replicas[ReplicaID(id)]
		if !ok {
			logger.Warn("got replica info failed.", "replicaId", id)
			continue
		}
		wg.Add(1)
		go func(pc *pb.PartCert, verifier Verifier) {
			if ok, err := verifier.Verify(pc.Signature, qc.BlockHash); err == nil && ok {
				atomic.AddUint64(&numVerified, 1)
			} else {
				logger.Warn("verify quorum cert signature failed.", "replicaId", id)
			}
			wg.Done()
		}(pc, info.Verifier)
	}
	wg.Wait()

	return numVerified >= uint64(utils.GetQuorumSize(rc.Metadata))
}

// VerifyVote will verify a vote from public keys stored in ReplicaConf
func (rc *ReplicaConf) VerifyVote(vote *pb.Vote) bool {
	if vote == nil || vote.Cert == nil {
		return false
	}
	info, ok := rc.Replicas[ReplicaID(vote.Voter)]
	if !ok {
		logger.Warn("got replica info failed.", "replicaId", vote.Voter)
		return false
	}

	if ok, err := info.Verifier.Verify(vote.Cert.Signature, vote.BlockHash); err != nil || !ok {
		logger.Error("verify vote signature failed.", "replicaId", vote.Voter)
		return false
	}

	return true
}

func GetBlockHash(block *pb.Block) []byte {
	var toHash []byte
	toHash = append(toHash, block.ParentHash...)
	height := make([]byte, 8)
	binary.LittleEndian.PutUint64(height, uint64(block.Height))
	toHash = append(toHash, height...)
	// TODO justify 会被修改造成hash不一致
	// genesis node
	//if block.Justify != nil {
	//	qc, _ := proto.Marshal(block.Justify)
	//	toHash = append(toHash, qc...)
	//}
	toHash = append(toHash, block.Cmds...)
	hash := sha256.Sum256(toHash)
	return hash[:]
}
