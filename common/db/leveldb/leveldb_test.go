package leveldb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLevelDB(t *testing.T) {
	provider := NewProvider()
	lvldb := provider.GetDBHandle("test")
	err := lvldb.Put([]byte("alice"), []byte("10"))
	assert.NoError(t, err)

	val, err := lvldb.Get([]byte("alice"))
	assert.NoError(t, err)
	assert.Equal(t, []byte("10"), val.([]byte), "Got value must be the same.")

	err = lvldb.Delete([]byte("alice"))
	assert.NoError(t, err)
	_, err = lvldb.Get([]byte("alice"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error retrieving leveldb key")
}
