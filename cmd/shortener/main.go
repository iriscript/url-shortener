package main

import (
	"io"
	"log"
	"math/rand"
	"net/http"
	"sync"
)

const (
	serverAddr = "localhost:8080"
	idAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	idLength   = 8
)

type urlStore struct {
	mu sync.RWMutex
	m  map[string]string
}

func newURLStore() *urlStore {
	return &urlStore{m: make(map[string]string)}
}

func (s *urlStore) Save(url string) string {
	id := generateID()

	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		if _, exists := s.m[id]; !exists {
			break
		}
		id = generateID()
	}
	s.m[id] = url

	return id
}

func (s *urlStore) Get(id string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	url, ok := s.m[id]
	return url, ok
}

func generateID() string {
	b := make([]byte, idLength)
	for i := range b {
		b[i] = idAlphabet[rand.Intn(len(idAlphabet))]
	}
	return string(b)
}

func shortenHandler(store *urlStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) == 0 {
			http.Error(w, "empty or invalid request body", http.StatusBadRequest)
			return
		}

		id := store.Save(string(body))

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		_, err = w.Write([]byte("http://" + serverAddr + "/" + id))
		if err != nil {
			log.Printf("error while writes the data to the connection %v", err)
			return
		}
	}
}

func redirectHandler(store *urlStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		url, ok := store.Get(id)
		if !ok {
			http.Error(w, "unknown short URL id", http.StatusBadRequest)
			return
		}

		w.Header().Set("Location", url)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}
}

func newRouter(store *urlStore) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /{$}", shortenHandler(store))
	mux.HandleFunc("GET /{id}", redirectHandler(store))
	return mux
}

func main() {
	store := newURLStore()

	log.Printf("starting server at %s", serverAddr)
	log.Fatal(http.ListenAndServe(serverAddr, newRouter(store)))
}
