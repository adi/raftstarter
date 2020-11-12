// Package httpd provides the HTTP server for accessing the distributed key-value store.
// It also provides the endpoint for other nodes to join an existing cluster.
package httpd

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/hashicorp/go-hclog"
)

// Store is the interface Raft-backed key-value stores must implement.
type storeInterface interface {
	// Get returns the value for the given key.
	Get(key string) (interface{}, error)

	// Set sets the value for the given key, via distributed consensus.
	Set(key string, value interface{}) error

	// Delete removes the given key, via distributed consensus.
	Delete(key string) error
}

// Service provides HTTP service.
type Service struct {
	addr string
	ln   net.Listener

	store  storeInterface
	logger hclog.Logger
}

// New returns an uninitialized HTTP service.
func New(addr string, store storeInterface, logger hclog.Logger) *Service {
	return &Service{
		addr:   addr,
		store:  store,
		logger: logger,
	}
}

// Start starts the service.
func (s *Service) Start() error {
	server := http.Server{
		Handler: s,
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln

	http.Handle("/", s)

	go func() {
		err := server.Serve(s.ln)
		if err != nil {
			log.Fatalf("HTTP serve: %s", err)
		}
	}()

	return nil
}

// Close closes the service.
func (s *Service) Close() {
	s.ln.Close()
	return
}

// ServeHTTP allows Service to serve HTTP requests.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/key") {
		s.handleKeyRequest(w, r)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *Service) handleKeyRequest(w http.ResponseWriter, r *http.Request) {
	getKey := func() string {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 3 {
			return ""
		}
		return parts[2]
	}

	switch r.Method {
	case "GET":
		k := getKey()
		if k == "" {
			s.logger.Debug("empty key", "k", k)
			w.WriteHeader(http.StatusBadRequest)
		}
		v, err := s.store.Get(k)
		if err != nil {
			s.logger.Debug("store get", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		b, err := json.Marshal(map[string]interface{}{k: v})
		if err != nil {
			s.logger.Debug("encode json", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		io.WriteString(w, string(b))

	case "POST":
		// Read the value from the POST body.
		m := map[string]interface{}{}
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			s.logger.Debug("decode json", "err", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for k, v := range m {
			if err := s.store.Set(k, v); err != nil {
				s.logger.Debug("store set", "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

	case "DELETE":
		k := getKey()
		if k == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := s.store.Delete(k); err != nil {
			s.logger.Debug("store del", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		s.store.Delete(k)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
	return
}

// Addr returns the address on which the Service is listening
func (s *Service) Addr() net.Addr {
	return s.ln.Addr()
}
