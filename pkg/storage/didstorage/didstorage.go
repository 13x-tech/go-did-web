package didstorage

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/13x-tech/go-did-web/pkg/didweb"
	"github.com/TBD54566975/ssi-sdk/did"
)

type Storage interface {
	Set(id string, value []byte) error
	Get(id string) ([]byte, error)
	Delete(id string) error
}

func NewDIDStore(storage Storage) *DIDStore {
	return &DIDStore{storage}
}

type DIDStore struct {
	store Storage
}

type KeyInput struct {
	Purposes           []string               `json:"purposes"`
	VerificationMethod did.VerificationMethod `json:"verificationMethod"`
}

func (d *DIDStore) Register(id string, keys []KeyInput, services []did.Service) (*did.Document, error) {
	newDID, err := didweb.New(id)
	if err != nil {
		return nil, err
	}

	doc := did.NewDIDDocumentBuilder()
	doc.Document = newDID
	for _, key := range keys {
		key.VerificationMethod.Controller = doc.ID
		if err := doc.AddVerificationMethod(key.VerificationMethod); err != nil {
			return nil, fmt.Errorf("verification method error: %w", err)
		}
		for _, purpose := range key.Purposes {
			if strings.EqualFold(purpose, "authentication") {
				if err := doc.AddAuthenticationMethod("#" + key.VerificationMethod.ID); err != nil {
					return nil, fmt.Errorf("could not add authentication method: %w", err)
				}
			} else if strings.EqualFold(purpose, "assertionMethod") {
				if err := doc.AddAssertionMethod("#" + key.VerificationMethod.ID); err != nil {
					return nil, fmt.Errorf("could not add assertion method: %w", err)
				}
			} else if strings.EqualFold(purpose, "capabilityDelegation") {
				if err := doc.AddCapabilityDelegation("#" + key.VerificationMethod.ID); err != nil {
					return nil, fmt.Errorf("could not add capability delegation: %w", err)
				}
			} else if strings.EqualFold(purpose, "capabilityInvocation") {
				if err := doc.AddCapabilityInvocation("#" + key.VerificationMethod.ID); err != nil {
					return nil, fmt.Errorf("could not add capbility invocation: %w", err)
				}
			} else if strings.EqualFold(purpose, "keyAgreement") {
				if err := doc.AddKeyAgreement("#" + key.VerificationMethod.ID); err != nil {
					return nil, fmt.Errorf("could not add key agreement: %w", err)
				}
			}
		}
	}

	if len(doc.AssertionMethod) == 0 {
		return nil, fmt.Errorf("did document must have at least one assertion verifiction method")
	}

	for _, service := range services {
		if err := doc.AddService(service); err != nil {
			return nil, fmt.Errorf("service error: %w", err)
		}
	}

	bytes, err := json.Marshal(newDID)
	if err != nil {
		return nil, err
	}
	if err := d.store.Set(id, bytes); err != nil {
		return nil, err
	}

	return newDID, nil
}

func (d *DIDStore) Resolve(id string) (*did.Document, error) {
	bytes, err := d.store.Get(id)
	if err != nil {
		return nil, fmt.Errorf("could not get from store: %w", err)
	} else if len(bytes) == 0 {
		return nil, fmt.Errorf("not found")
	}
	var doc did.Document
	if err := json.Unmarshal(bytes, &doc); err != nil {
		return nil, fmt.Errorf("could not parse: %w", err)
	}
	return &doc, nil
}

func (d *DIDStore) Delete(id string) error {
	return d.store.Delete(id)
}
