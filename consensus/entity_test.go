package consensus

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zhigui-projects/go-hotstuff/pb"
)

func TestGetBlockHash(t *testing.T) {
	hash := GetBlockHash(&pb.Block{
		Height: 0,
	})
	assert.Equal(t, "af5570f5a1810b7af78caf4bc70a660f0df51e42baf91d4de5b2328de0e83dfc", hex.EncodeToString(hash))
}
