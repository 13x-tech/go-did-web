package didweb

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Testing New()
func TestNew(t *testing.T) {
	id := "example.com:alice"
	doc, err := New(id)

	assert.NoError(t, err)
	assert.NotNil(t, doc)
	assert.Equal(t, "did:web:example.com:alice", doc.ID)
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
		{"did:web:example.com", "did:web:example.com", true},
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
