/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package consensus

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"sync/atomic"

	"github.com/zhigui-projects/go-hotstuff/api"
	"github.com/zhigui-projects/go-hotstuff/common/utils"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

type ReplicaID int64

// ReplicaInfo holds information about a replica
type ReplicaInfo struct {
	Verifier api.Verifier
}

type ReplicaConf struct {
	Metadata *pb.ConfigMetadata
	Replicas map[ReplicaID]*ReplicaInfo
	Logger   api.Logger
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
			rc.Logger.Warning("got replica info failed.", "replicaId", id)
			continue
		}
		wg.Add(1)
		go func(pc *pb.PartCert, verifier api.Verifier) {
			if ok, err := verifier.Verify(pc.Signature, qc.BlockHash); err == nil && ok {
				atomic.AddUint64(&numVerified, 1)
			} else {
				rc.Logger.Warning("verify quorum cert signature failed.", "replicaId", id)
			}
			wg.Done()
		}(pc, info.Verifier)
	}
	wg.Wait()

	return numVerified >= uint64(utils.GetQuorumSize(rc.Metadata))
}

// VerifyVote will verify a vote from public keys stored in ReplicaConf
func (rc *ReplicaConf) VerifyIdentity(replicaId ReplicaID, signature, digest []byte) bool {
	info, ok := rc.Replicas[replicaId]
	if !ok {
		rc.Logger.Error("got replica info failed.", "replicaId", replicaId)
		return false
	}
	if ok, err := info.Verifier.Verify(signature, digest); err != nil || !ok {
		rc.Logger.Error("verify vote signature failed.", "replicaId", replicaId)
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
