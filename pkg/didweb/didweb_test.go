package didweb

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/assert"
)

func GenSchnorrPubKey() *secp256k1.PublicKey {
	private, _ := secp256k1.GeneratePrivateKey()
	return private.PubKey()
}

// compressPublicKey compresses an ECDSA public key.
func compressPublicKey(pubKey *ecdsa.PublicKey) []byte {
	byteLen := (pubKey.Params().BitSize + 7) >> 3
	compressed := make([]byte, 1+byteLen)
	compressed[0] = 2 // 02/03 prefix depending on y's least significant bit
	if pubKey.Y.Bit(0) == 1 {
		compressed[0] = 3
	}

	xBytes := pubKey.X.Bytes()
	copy(compressed[1+byteLen-len(xBytes):], xBytes)
	return compressed
}

func GenP256Key() ([]byte, error) {
	pKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("could not generate key: %w", err)
	}
	return compressPublicKey(&pKey.PublicKey), nil
}

// Testing New()
func TestNew(t *testing.T) {
	id := "example.com:alice"
	pubKey, err := GenP256Key()
	if err != nil {
		t.Fatal(err)
	}
	doc, err := New(id, pubKey)

	assert.NoError(t, err)
	assert.NotNil(t, doc)
	assert.Equal(t, "did:web:example.com:alice", doc.ID)
	//TODO assert Key
}

func TestParsePath(t *testing.T) {
	tt := []struct {
		input     string
		expected  string
		expectErr bool
	}{
		{"example.com/.well-known/did.json", "did:web:example.com", false},
		{"example.com/john/did.json", "did:web:example.com:john", false},
		{"example.com/accounting/john/did.json", "did:web:example.com:accounting:john", false},
		{"example.com", "", true},
	}

	for i, tc := range tt {
		t.Run(fmt.Sprintf("parse url case: %d", i+1), func(t *testing.T) {
			res, err := ParsePath(tc.input)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, res.DID())
			}
		})
	}
}

// Testing Parse()
func TestParse(t *testing.T) {
	tt := []struct {
		input     string
		expected  string
		expectErr bool
	}{
		{"did:web:example.com", "example.com", false},
		{"did:web:example.com:john", "example.com:john", false},
		{"example.com", "", true},
		// add more cases as needed
	}

	for i, tc := range tt {
		t.Run(fmt.Sprintf("parse case: %d", i+1), func(t *testing.T) {
			res, err := Parse(tc.input)

			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, res.ID())
			}
		})
	}
}
