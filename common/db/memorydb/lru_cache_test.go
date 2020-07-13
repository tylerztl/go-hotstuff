package memorydb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLruCache(t *testing.T) {
	lru := NewLRUCache(2)
	err := lru.Put("alice", 10)
	assert.NoError(t, err)

	val, err := lru.Get("alice")
	assert.NoError(t, err)
	assert.Equal(t, 10, val, "Got value must be the same.")

	err = lru.Put("bob", 11)
	assert.NoError(t, err)
	err = lru.Put("carol", 12)
	_, err = lru.Get("alice")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Error retrieving memorydb key: alice")

	err = lru.Delete("carol")
	assert.NoError(t, err)
	_, err = lru.Get("carol")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Error retrieving memorydb key: carol")
}
