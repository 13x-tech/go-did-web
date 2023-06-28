package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/13x-tech/go-did-web/pkg/didweb"
	"github.com/13x-tech/go-did-web/pkg/storage"
	"github.com/13x-tech/go-did-web/pkg/storage/didstorage"
	"github.com/TBD54566975/ssi-sdk/did"
	"github.com/gorilla/mux"
)

type Store interface {
	Register(id string, keys []didstorage.KeyInput, services []did.Service) (*did.Document, error)
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

type Server struct {
	host    string
	port    int
	domain  string
	store   Store
	handler http.Handler
}

func New(opts ...Option) (*Server, error) {
	s := &Server{}
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return s, err
		}
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

	if s.handler == nil {
		r := mux.NewRouter()
		r.HandleFunc("/register", s.keyAuthMiddleware(s.handleRegister)).Methods("POST")
		r.HandleFunc("/resolve/{id}", s.handleResolve).Methods("GET")
		r.HandleFunc("/update/{id}", s.handleUpdate).Methods("POST")
		r.HandleFunc("/delete/{id}", s.handleDelete).Methods("DELETE")
		r.HandleFunc("/health", s.handleHealth).Methods("GET")
		r.HandleFunc("/.well-known", s.handleWellKnownDir).Methods("GET")

		r.PathPrefix("/").HandlerFunc(s.handleDefault).Methods("GET")
		s.handler = r
	}

	return s, nil
}

func (s *Server) Start() error {
	return http.ListenAndServe(fmt.Sprintf("%s:%d", s.host, s.port), s.handler)
}

func (s *Server) handleWellKnownDir(w http.ResponseWriter, r *http.Request) {
	log.Printf("Well Known: %s\n", r.URL.Path)
	w.WriteHeader(http.StatusNotImplemented)
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

	doc, err := s.store.Register(input.ID, input.Keys, input.Services)
	if err != nil {
		s.errorResponse(w, 500, fmt.Sprintf("could not register: %s", err.Error()))
		return
	}

	s.jsonSuccess(w, doc)
}

func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.RawPath, "/")
	if len(pathParts) < 3 {
		fmt.Printf("path parts: %s\n", pathParts)
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
			fmt.Printf("no keys")
		}
		next.ServeHTTP(w, r)
	})
}

type RegisterRequest struct {
	ID       string                `json:"id"`
	Keys     []didstorage.KeyInput `json:"keys"`
	Services []did.Service         `json:"services"`
}
