/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifyECDSA(t *testing.T) {
	t.Parallel()

	// Generate a key
	lowLevelKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	assert.NoError(t, err)

	msg := []byte("hello world")
	sigma, err := Sign(lowLevelKey, msg)
	assert.NoError(t, err)

	valid, err := Verify(&lowLevelKey.PublicKey, sigma, msg)
	assert.NoError(t, err)
	assert.True(t, valid)

	_, err = Verify(&lowLevelKey.PublicKey, nil, msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed unmashalling signature [")

	R, S, err := UnmarshalECDSASignature(sigma)
	assert.NoError(t, err)
	S.Add(GetCurveHalfOrdersAt(elliptic.P256()), big.NewInt(1))
	sigmaWrongS, err := MarshalECDSASignature(R, S)
	assert.NoError(t, err)
	_, err = Verify(&lowLevelKey.PublicKey, sigmaWrongS, msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid S. Must be smaller than half the order [")
}
