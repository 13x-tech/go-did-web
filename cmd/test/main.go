package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/13x-tech/go-did-web/pkg/didweb"
)

func main() {
	dids := []string{
		"did:web:example.com",
		"did:web:localhost%3A8443",
		"did:web:example.com:path:some%2Bsubpath",
		"did:web:example.com:path:some%2Bsubpath?key=123",
		"did:web:example.com:user:alice",
		"did:web:did.actor:alice",
		"did:web:did.actor:bob",
		"did:web:dwn.tbddev.org",
	}
	for _, id := range dids {
		if parsed, err := didweb.Parse(id); err == nil {
			fmt.Printf("%s -> (%s)\n", id, parsed.DID())
			results, err := didweb.Resolve(parsed.DID(), http.DefaultClient)
			if err != nil {
				fmt.Printf("\t -> [unknown error] - %s\n\n", err.Error())
				continue
			}
			jsonResults, err := json.Marshal(results)
			if err != nil {
				fmt.Printf("\t -> [could not format] - %s\n\n", err.Error())
				continue
			}

			fmt.Printf("\t -> %s\n\n", jsonResults)
		} else {
			fmt.Printf("%s\n", id)
			fmt.Printf("\t -> [unknown error] - %s\n\n", err.Error())
		}
	}
}
