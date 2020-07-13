/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

type Verifier interface {
	Verify(signature, digest []byte) (bool, error)
}

type Signer interface {
	Sign(digest []byte) ([]byte, error)
}