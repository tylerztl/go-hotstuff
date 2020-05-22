package consensus

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"sync/atomic"

	"github.com/golang/protobuf/proto"
	"github.com/zhigui-projects/go-hotstuff/common/log"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

var logger = log.GetLogger()

type ReplicaID int64

type Verifier interface {
	Verify(signature, digest []byte) (bool, error)
}

// ReplicaInfo holds information about a replica
type ReplicaInfo struct {
	ID       ReplicaID
	Address  string
	Verifier Verifier
}

type ReplicaConf struct {
	QuorumSize int
	Replicas   map[ReplicaID]*ReplicaInfo
}

// VerifyQuorumCert will verify a QuorumCert from public keys stored in ReplicaConfig
func (rc *ReplicaConf) VerifyQuorumCert(qc *pb.QuorumCert) bool {
	if qc == nil || len(qc.Signs) < rc.QuorumSize {
		return false
	}
	var wg sync.WaitGroup
	var numVerified uint64 = 0
	for id, pc := range qc.Signs {
		info, ok := rc.Replicas[ReplicaID(id)]
		if !ok {
			logger.Warn("got signature from replicas failed.", "replicaId", id)
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
	return numVerified >= uint64(rc.QuorumSize)
}

func GetBlockHash(block *pb.Block) []byte {
	var toHash []byte
	toHash = append(toHash, block.ParentHash...)
	height := make([]byte, 8)
	binary.LittleEndian.PutUint64(height, uint64(block.Height))
	toHash = append(toHash, height...)
	// genesis node
	if block.Justify != nil {
		qc, _ := proto.Marshal(block.Justify)
		toHash = append(toHash, qc...)
	}
	toHash = append(toHash, block.Cmds...)
	hash := sha256.Sum256(toHash)
	return hash[:]
}
