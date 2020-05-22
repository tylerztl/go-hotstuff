package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
)

type ECDSASignature struct {
	R, S *big.Int
}

var (
	// curveHalfOrders contains the precomputed curve group orders halved.
	// It is used to ensure that signature' S value is lower or equal to the
	// curve group order halved. We accept only low-S signatures.
	// They are precomputed for efficiency reasons.
	curveHalfOrders = map[elliptic.Curve]*big.Int{
		elliptic.P224(): new(big.Int).Rsh(elliptic.P224().Params().N, 1),
		elliptic.P256(): new(big.Int).Rsh(elliptic.P256().Params().N, 1),
		elliptic.P384(): new(big.Int).Rsh(elliptic.P384().Params().N, 1),
		elliptic.P521(): new(big.Int).Rsh(elliptic.P521().Params().N, 1),
	}
)

func Sign(k *ecdsa.PrivateKey, digest []byte) ([]byte, error) {
	r, s, err := ecdsa.Sign(rand.Reader, k, digest)
	if err != nil {
		return nil, err
	}

	s, _, err = toLowS(&k.PublicKey, s)
	if err != nil {
		return nil, err
	}

	return MarshalECDSASignature(r, s)
}

func Verify(k *ecdsa.PublicKey, signature, digest []byte) (bool, error) {
	r, s, err := UnmarshalECDSASignature(signature)
	if err != nil {
		return false, fmt.Errorf("failed unmashalling signature [%s]", err)
	}

	lowS, err := isLowS(k, s)
	if err != nil {
		return false, err
	}

	if !lowS {
		return false, fmt.Errorf("invalid S. Must be smaller than half the order [%s][%s]", s, GetCurveHalfOrdersAt(k.Curve))
	}

	return ecdsa.Verify(k, digest, r, s), nil
}

func GetCurveHalfOrdersAt(c elliptic.Curve) *big.Int {
	return big.NewInt(0).Set(curveHalfOrders[c])
}

func MarshalECDSASignature(r, s *big.Int) ([]byte, error) {
	return asn1.Marshal(ECDSASignature{r, s})
}

func UnmarshalECDSASignature(raw []byte) (*big.Int, *big.Int, error) {
	// Unmarshal
	sig := new(ECDSASignature)
	_, err := asn1.Unmarshal(raw, sig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed unmashalling signature [%s]", err)
	}

	// Validate sig
	if sig.R == nil {
		return nil, nil, errors.New("invalid signature, R must be different from nil")
	}
	if sig.S == nil {
		return nil, nil, errors.New("invalid signature, S must be different from nil")
	}

	if sig.R.Sign() != 1 {
		return nil, nil, errors.New("invalid signature, R must be larger than zero")
	}
	if sig.S.Sign() != 1 {
		return nil, nil, errors.New("invalid signature, S must be larger than zero")
	}

	return sig.R, sig.S, nil
}

// IsLow checks that s is a low-S
func isLowS(k *ecdsa.PublicKey, s *big.Int) (bool, error) {
	halfOrder, ok := curveHalfOrders[k.Curve]
	if !ok {
		return false, fmt.Errorf("curve not recognized [%s]", k.Curve)
	}

	return s.Cmp(halfOrder) != 1, nil
}

func toLowS(k *ecdsa.PublicKey, s *big.Int) (*big.Int, bool, error) {
	lowS, err := isLowS(k, s)
	if err != nil {
		return nil, false, err
	}
	if !lowS {
		// Set s to N - s that will be then in the lower part of signature space
		// less or equal to half order
		s.Sub(k.Params().N, s)
		return s, true, nil
	}

	return s, false, nil
}
