package didweb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/TBD54566975/ssi-sdk/crypto"
	"github.com/TBD54566975/ssi-sdk/did"
	"github.com/TBD54566975/ssi-sdk/did/web"
)

func New(id string, publicKey []byte) (*did.Document, error) {
	dweb := web.DIDWeb(fmt.Sprintf("did:web:%s", id))
	return dweb.CreateDoc(crypto.P256, publicKey)
}

type DIDWebURL struct {
	host        string
	parts       []string
	QueryParams url.Values
	Anchor      string
}

func (u *DIDWebURL) URL() string {
	parts := u.parts
	if len(parts) == 0 {
		parts = []string{".well-known"}
	}

	rawURL, err := url.Parse(fmt.Sprintf("https://%s/%s/did.json", u.Host(), strings.Join(parts, "/")))
	if err != nil {
		return ""
	}
	return rawURL.String()
}

func (u DIDWebURL) RawHost() string {
	return u.host
}

func (u DIDWebURL) Host() string {
	host := u.host
	port := 0
	decodedHost, err := url.QueryUnescape(host)
	if err != nil {
		return host
	}

	if strings.Contains(decodedHost, ":") {
		split := strings.Split(decodedHost, ":")
		if len(split) > 1 {
			var err error
			port, err = strconv.Atoi(split[1])
			if err != nil {
				return host
			}
			host = split[0]
		}
	}

	if port > 0 {
		return fmt.Sprintf("%s:%d", host, port)
	}

	return host
}

func (u *DIDWebURL) DID() string {
	return fmt.Sprintf("did:web:%s", u.ID())
}

func (u *DIDWebURL) ID() string {
	parts := u.parts
	if len(parts) == 0 {
		return u.host
	}
	for i, part := range parts {
		parts[i] = url.QueryEscape(part)
	}
	return fmt.Sprintf("%s:%s", u.host, strings.Join(parts, ":"))
}

func Parse(id string) (DIDWebURL, error) {
	didParts := strings.Split(id, ":")
	if len(didParts) < 3 {
		return DIDWebURL{}, fmt.Errorf("invalid did, must be in format did:web:example.org:john")
	}

	if didParts[0] != "did" || didParts[1] != "web" {
		return DIDWebURL{}, fmt.Errorf("invalid did, must be in format did:web:example.org:john")
	}
	d := DIDWebURL{
		host: didParts[2],
	}

	path := []string{}
	if len(didParts) > 3 {
		for i, part := range didParts {
			if i > 2 && len(part) > 0 {
				unescape, err := url.QueryUnescape(part)
				if err != nil {
					return DIDWebURL{}, fmt.Errorf("invalid unescape of path part")
				}
				path = append(path, unescape)
			}
		}
	}

	didURL, err := url.Parse(fmt.Sprintf("https://%s/%s", d.Host(), strings.Join(path, "/")))
	if err != nil {
		return DIDWebURL{}, fmt.Errorf("invalid did, must be in format did:web:example.org:john: %w", err)
	}
	if len(path) > 0 {
		d.parts = strings.Split(strings.Trim(didURL.Path, "/"), "/")
	}
	d.Anchor = didURL.Fragment
	d.QueryParams = didURL.Query()
	return d, nil
}

func ParsePath(path string) (DIDWebURL, error) {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return DIDWebURL{}, fmt.Errorf("invalid")
	}
	lastPart := parts[len(parts)-1]
	if !strings.EqualFold(lastPart, "did.json") {
		return DIDWebURL{}, fmt.Errorf("invalid")
	}
	parts = parts[:len(parts)-1]
	if strings.EqualFold(parts[len(parts)-1], ".well-known") {
		parts = parts[:len(parts)-1]
	}

	return Parse(fmt.Sprintf("did:web:%s", strings.Join(parts, ":")))
}

func Resolve(id string, client *http.Client) (*did.Document, error) {
	url, err := Parse(id)
	if err != nil {
		return nil, fmt.Errorf("could not parse did url: %w", err)
	}
	resp, err := client.Get(url.URL())
	if err != nil {
		return nil, fmt.Errorf("could not get did json: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrorDIDNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid status cod: %d - %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read body: %w", err)
	}
	var doc did.Document
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("could not decode document body: %w", err)
	}
	if !strings.EqualFold(id, doc.ID) {
		return nil, fmt.Errorf("masmatched document id: %w", err)
	}
	return &doc, nil
}

func Test() {
}

var (
	ErrorDIDNotFound = fmt.Errorf("not found")
)
