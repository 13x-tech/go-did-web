package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/13x-tech/go-did-web/pkg/didweb"
	"github.com/13x-tech/go-did-web/pkg/storage"
	"github.com/13x-tech/go-did-web/pkg/storage/didstorage"
	"github.com/TBD54566975/ssi-sdk/did"
	"github.com/gorilla/mux"
	"github.com/multiformats/go-multibase"
)

type Store interface {
	Register(doc *did.Document) error
	Resolve(id string) (*did.Document, error)
	Delete(id string) error
}

func NewStore(domain, storageDir, bucket string) (Store, error) {
	store, err := storage.New(storageDir, bucket)
	if err != nil {
		return nil, err
	}

	return didstorage.NewDIDStore(store), nil
}

type Message struct {
	id      string
	message string
}

func NewBroker() *PaymentBroker {
	return &PaymentBroker{
		mu:       sync.RWMutex{},
		clients:  make(map[string]map[chan string]struct{}),
		messages: make(chan Message),
	}
}

type PaymentBroker struct {
	mu       sync.RWMutex
	clients  map[string]map[chan string]struct{}
	messages chan Message
}

func (b *PaymentBroker) Start() {
	go func() {
		for {
			select {
			case msg := <-b.messages:
				b.mu.RLock()
				clients, ok := b.clients[msg.id]
				b.mu.RUnlock()
				if ok {
					for c := range clients {
						c <- msg.message
					}
				}
			}
		}
	}()
}

func (b *PaymentBroker) BroadcastPayment(id string) {
	fmt.Printf("attempt broadcast: %s", id)
	b.mu.RLock()
	client, ok := b.clients[id]
	b.mu.RUnlock()
	if ok {
		for c := range client {
			c <- "paid"
		}
		//TODO close out connections?
	}
}

func (b *PaymentBroker) WaitForPayment(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported!", http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]
	fmt.Printf("Connected and waiting: %s", id)
	b.mu.Lock()
	clients, ok := b.clients[id]
	if !ok {
		clients = make(map[chan string]struct{})
	}
	messageChan := make(chan string)
	clients[messageChan] = struct{}{}
	b.clients[id] = clients
	b.mu.Unlock()

	ctx := r.Context()
	go func() {
		<-ctx.Done()
		b.mu.Lock()
		clients, ok := b.clients[id]
		if ok {
			delete(clients, messageChan)
			b.clients[id] = clients
		}
		b.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for {
		select {
		case msg := <-messageChan:
			fmt.Fprintf(w, "data: Message: %s\n\n", msg)
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

type Option func(s *Server) error

func WithHost(host string) Option {
	return func(s *Server) error {
		s.host = host
		return nil
	}
}

func WithPort(port int) Option {
	return func(s *Server) error {
		s.port = port
		return nil
	}
}

func WithDomain(domain string) Option {
	return func(s *Server) error {
		s.domain = domain
		return nil
	}
}

func WithStore(store Store) Option {
	return func(s *Server) error {
		s.store = store
		return nil
	}
}

func WithHandler(handler http.Handler) Option {
	return func(s *Server) error {
		s.handler = handler
		return nil
	}
}

func WithRegisterStore(store *didstorage.RegisterStore) Option {
	return func(s *Server) error {
		s.regStore = store
		return nil
	}
}

type Server struct {
	host      string
	port      int
	domain    string
	store     Store
	regStore  *didstorage.RegisterStore
	payBroker *PaymentBroker
	handler   http.Handler
}

func New(opts ...Option) (*Server, error) {
	s := &Server{}
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return s, err
		}
	}

	if s.regStore == nil {
		return nil, fmt.Errorf("reg store required")
	}

	// Do some sort of cert check
	if len(s.domain) == 0 {
		return nil, fmt.Errorf("invalid domain")
	}

	if s.host == "" {
		s.host = "0.0.0.0"
	}

	if s.port == 0 {
		s.port = 8080
	}
	s.payBroker = NewBroker()
	go s.payBroker.Start()
	if s.handler == nil {
		r := mux.NewRouter()
		r.HandleFunc("/register", s.addCORS(false, s.handleRegister))
		r.HandleFunc("/paid/{id}", s.addCORS(false, s.handlePaid))
		r.HandleFunc("/payment/{id}", s.addCORS(false, s.payBroker.WaitForPayment))
		r.HandleFunc("/resolve/{id}", s.addCORS(false, s.handleResolve)).Methods("GET")
		r.HandleFunc("/update/{id}", s.addCORS(true, s.handleUpdate)).Methods("POST")
		r.HandleFunc("/delete/{id}", s.addCORS(true, s.handleDelete)).Methods("DELETE")
		r.HandleFunc("/health", s.addCORS(true, s.handleHealth)).Methods("GET")
		r.PathPrefix("/.well-known").HandlerFunc(s.addCORS(false, s.handleWellKnownDir)).Methods("GET")
		s.handler = r
	}

	return s, nil
}

func (s *Server) Start() error {
	return http.ListenAndServe(fmt.Sprintf("%s:%d", s.host, s.port), s.handler)
}

func (s *Server) handleWellKnownDir(w http.ResponseWriter, r *http.Request) {
	log.Printf("Well Known: %s\n", r.URL.Path)
	if strings.EqualFold(r.URL.Path, ".well-known/nostr.json") {
		s.handleWellKnownNostr(w, r)
		return
	}
	//TODO DID well-knowns

	w.WriteHeader(http.StatusNotImplemented)
}

type NostrWellKnown struct {
	Names map[string]string
}

func (s *Server) handleWellKnownNostr(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if len(name) == 0 {
		s.jsonSuccess(w, NostrWellKnown{Names: map[string]string{}})
		return
	}

	doc, err := s.store.Resolve(fmt.Sprintf("%s:%s", s.domain, name))
	if err != nil {
		s.jsonSuccess(w, NostrWellKnown{Names: map[string]string{}})
		return
	}

	for _, vm := range doc.VerificationMethod {
		if strings.EqualFold(vm.Type.String(), "SchnorrSecp256k1VerificationKey2019") && strings.Contains(strings.ToLower(vm.ID), "nostr") {
			enc, data, err := multibase.Decode(vm.PublicKeyMultibase)
			if err != nil {
				s.jsonSuccess(w, NostrWellKnown{Names: map[string]string{}})
				return
			}
			if enc != multibase.Base16 {
				s.jsonSuccess(w, NostrWellKnown{Names: map[string]string{}})
				return
			}
			s.jsonSuccess(w, NostrWellKnown{Names: map[string]string{
				name: fmt.Sprintf("%x", data),
			}})
			return
		}
	}

	s.jsonSuccess(w, NostrWellKnown{Names: map[string]string{}})
}

func (s *Server) handleDefault(w http.ResponseWriter, r *http.Request) {
	path := fmt.Sprintf("%s/%s", url.QueryEscape(r.Host), r.URL.Path)
	url, err := didweb.ParsePath(path)
	if err != nil {
		s.errorResponse(w, 404, "not found")
		return
	}
	doc, err := s.store.Resolve(url.ID())
	if err != nil {
		fmt.Printf("could not resolve %s: %s\n", url.ID(), err.Error())
		s.errorResponse(w, 404, "not found")
		return
	}
	s.jsonSuccess(w, doc)
}

type PayInfo struct {
	PaymentHash string `json:"payment_hash"`
	Amount      int    `json:"amount"`
}

func (s *Server) handlePaid(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, ok := vars["id"]
	if !ok {
		fmt.Printf("pay enpoint no id\n")
		return
	}

	doc, err := s.regStore.Paid(id)
	if err != nil {
		s.errorResponse(w, 401, "unauthorized")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("invalid body: %s\n", err.Error())
		return
	}

	var info PayInfo
	if err := json.Unmarshal(body, &info); err != nil {
		fmt.Printf("could not get payment info: %s\n", err.Error())
		return
	}

	if err := s.store.Register(doc); err != nil {
		s.errorResponse(w, 500, fmt.Sprintf("could not register: %s", err.Error()))
		return
	}

	go s.payBroker.BroadcastPayment(doc.ID)
	s.jsonSuccess(w, "ok")
}
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) errorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(code)
	errMessage, err := json.Marshal(struct {
		Error string `json:"error"`
	}{Error: message})
	if err != nil {
		errMessage = []byte("{ error: 'unknown' }")
	}
	w.Write(errMessage)
}

func (s *Server) jsonSuccess(w http.ResponseWriter, response any) {
	bytes, err := json.Marshal(response)
	if err != nil {
		s.errorResponse(w, 500, fmt.Sprintf("could not parse reponse: %s", err.Error()))
		return
	}

	w.Header().Add("Conent-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(bytes)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.errorResponse(w, 500, "could not get boxy")
		return
	}
	var input RegisterRequest
	if err := json.Unmarshal(body, &input); err != nil {
		s.errorResponse(w, 400, "invalid request")
		return
	}

	parts := strings.Split(input.ID, ":")
	if len(parts) < 2 {
		s.errorResponse(w, 400, fmt.Sprintf("id must be in the format of %s:sally, where sally is the name you're registering", s.domain))
		return
	}
	if parts[0] != s.domain {
		s.errorResponse(w, 400, fmt.Sprintf("invalid domain must be in the form if %s:sally, where sally is the name you're reistering", s.domain))
		return
	}

	if doc, err := s.store.Resolve(input.ID); err == nil && doc != nil {
		s.errorResponse(w, 400, "did exists")
		return
	}

	doc, err := didstorage.DIDFromProps(input.ID, input.Keys, input.Services)
	if err != nil {
		s.errorResponse(w, 500, fmt.Sprintf("could not register: %s", err.Error()))
		return
	}

	if payReq, ok := s.regStore.Get(doc); ok {
		s.jsonSuccess(w, payReq)
	} else {
		paymentRequest, err := s.regStore.Register(doc)
		if err != nil {
			s.errorResponse(w, 500, fmt.Sprintf("could not get payment request: %s", err.Error()))
			return
		}
		s.jsonSuccess(w, paymentRequest.PaymentRequest)
	}
}

func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.RequestURI, "/")
	if len(pathParts) < 3 {
		s.errorResponse(w, 400, "invalid")
		return
	}
	id := pathParts[2]
	if len(id) == 0 {
		s.errorResponse(w, 400, "invalid id")
		return
	}
	url, err := didweb.Parse(id)
	if err != nil {
		s.errorResponse(w, 400, "invalid id")
		return
	}

	if strings.EqualFold(url.RawHost(), s.domain) {
		if doc, err := s.store.Resolve(url.ID()); err == nil {
			s.jsonSuccess(w, doc)
			return
		}
	} else {
		if doc, err := didweb.Resolve(url.DID(), http.DefaultClient); err == nil {
			s.jsonSuccess(w, doc)
			return
		}
	}

	s.errorResponse(w, 404, "not found")
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {}

func (s *Server) keyAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys, ok := r.Header["X-Api-Key"]
		if !ok || len(keys) == 0 {
			s.errorResponse(w, 401, "unauthorized")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) addCORS(limited bool, next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limited {
			w.Header().Set("Access-Control-Allow-Origin", s.domain)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type RegisterRequest struct {
	ID       string                `json:"id"`
	Keys     []didstorage.KeyInput `json:"keys"`
	Services []did.Service         `json:"services"`
}
