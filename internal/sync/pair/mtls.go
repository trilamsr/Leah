package pair

import (
	"crypto/tls"
	"errors"
)

func NewMTLSConfig(sharedKey [32]byte) (*tls.Config, error) {
	return &tls.Config{MinVersion: tls.VersionTLS13}, nil
}

var ErrUnpinnedCert = errors.New("pair: peer cert fingerprint does not match shared key")

func PinFingerprint(sharedKey [32]byte) [32]byte { return [32]byte{} }
