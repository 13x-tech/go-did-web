package didstorage

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"

	"github.com/13x-tech/go-did-web/pkg/didweb"
	"github.com/TBD54566975/ssi-sdk/did"
)

type Storage interface {
	Set(id string, value []byte) error
	Get(id string) ([]byte, error)
	Delete(id string) error
}

func DIDFromProps(id string, keys []KeyInput, services []did.Service) (*did.Document, error) {
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

	return newDID, nil
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

func (d *DIDStore) Register(doc *did.Document) error {
	bytes, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("invalid doc: %w", err)
	}
	didwebUrl, err := didweb.Parse(doc.ID)
	if err != nil {
		return fmt.Errorf("could not parse did doc id: %w", err)
	}
	if err := d.store.Set(didwebUrl.ID(), bytes); err != nil {
		return fmt.Errorf("could not store: %w", err)
	}

	return nil
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

type RegisterStore struct {
	apiHost string
	apiKey  string
	store   Storage
}

func NewRegisterStore(apiHost, apiKey string, storage Storage) *RegisterStore {
	return &RegisterStore{
		apiHost: apiHost,
		apiKey:  apiKey,
		store:   storage,
	}
}

type PaymentResponse struct {
	PaymentHash    string `json:"payment_hash"`
	PaymentRequest string `json:"payment_request"`
}

func (s *RegisterStore) Get(doc *did.Document) (string, bool) {
	payReq, err := s.store.Get(doc.ID)
	if err != nil || len(payReq) == 0 {
		return "", false
	}

	if s.validatePaymentRequest(string(payReq)) {
		return string(payReq), true
	} else {
		fmt.Printf("Invalid Pay Req\n")
		return "", false
	}

}

func (s *RegisterStore) Paid(id string) (*did.Document, error) {
	docBytes, err := s.store.Get(id)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	var doc did.Document
	if err := json.Unmarshal(docBytes, &doc); err != nil {
		return nil, fmt.Errorf("invalid document: %w", err)
	}

	if err := s.store.Delete(id); err != nil {
		return nil, fmt.Errorf("could not delete secret: %w", err)
	}
	s.store.Delete(doc.ID)

	return &doc, nil
}

func (s *RegisterStore) Register(doc *did.Document) (*PaymentResponse, error) {

	if doc.ID == "" {
		return nil, fmt.Errorf("invalid did doc")
	}

	docJSON, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("could not marshal: %w", err)
	}
	nonce := make([]byte, 64)
	_, err = rand.Read(nonce)
	if err != nil {
		return nil, fmt.Errorf("could not generate randomess: %w", err)
	}

	request := struct {
		Out     bool   `json:"out"`
		Memo    string `json:"memo,omitempty"`
		Amount  int    `json:"amount"`
		Expiry  int    `json:"expiry,omitempty"`
		Unit    string `json:"unit,omitempty"`
		WebHook string `json:"webhook,omitempty"`
	}{
		Out:     false,
		Memo:    fmt.Sprintf("Register %s", doc.ID),
		Amount:  69,
		WebHook: fmt.Sprintf("https://did-web.onrender.com/paid/%x", nonce),
	}

	jsonRequest, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("https://%s/api/v1/payments", s.apiHost), strings.NewReader(string(jsonRequest)))
	if err != nil {
		return nil, err
	}
	req.Header.Add("X-Api-Key", s.apiKey)
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not do request: %w", err)
	}

	responseData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("invalid status code: %d - %s", resp.StatusCode, resp.Status)
	}

	var response PaymentResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		return nil, fmt.Errorf("could not parse: %w", err)
	}

	if err := s.store.Set(fmt.Sprintf("%x", nonce), docJSON); err != nil {
		return nil, fmt.Errorf("could not store payment request: %w", err)
	}

	if err := s.store.Set(doc.ID, []byte(response.PaymentRequest)); err != nil {
		return nil, fmt.Errorf("could not store payment request: %w", err)
	}

	return &response, nil
}

func (s *RegisterStore) validatePaymentRequest(payReq string) bool {
	jsonRequest, _ := json.Marshal(struct {
		Data string `json:"data"`
	}{Data: payReq})
	req, err := http.NewRequest("POST", fmt.Sprintf("https://%s/api/v1/payments", s.apiHost), strings.NewReader(string(jsonRequest)))
	if err != nil {
		return false
	}
	req.Header.Add("X-Api-Key", s.apiKey)
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}

	responseData, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	if resp.StatusCode != http.StatusOK {
		return false
	}

	if len(responseData) > 0 {

		fmt.Printf("Response Data: %s\n", responseData)
		return true
	}
	return false
}
