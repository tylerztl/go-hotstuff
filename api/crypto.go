package api

type Verifier interface {
	Verify(signature, digest []byte) (bool, error)
}

type Signer interface {
	Sign(digest []byte) ([]byte, error)
}