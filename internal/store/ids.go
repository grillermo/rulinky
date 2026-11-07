package store

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// newUUID returns a random (version 4) UUID string, the format the Rails app
// used for links.id and link_content_jobs.id. crypto/rand.Read never fails
// (Go 1.24+).
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// newAPIToken returns 24 random base58 characters, like Rails' has_secure_token.
func newAPIToken() (string, error) {
	out := make([]byte, 24)
	max := big.NewInt(int64(len(base58Alphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = base58Alphabet[n.Int64()]
	}
	return string(out), nil
}
