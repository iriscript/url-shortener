package main

import (
	"log"

	"github.com/iriscript/url-shortener/internal/handler"
	"github.com/iriscript/url-shortener/internal/repository"
	"github.com/iriscript/url-shortener/internal/server"
)

const serverAddr = "localhost:8080"

func main() {
	repo := repository.NewMemoryRepository()
	h := handler.NewURLHandler(repo, "http://"+serverAddr)
	router := server.NewRouter(h)
	srv := server.New(serverAddr, router)

	log.Printf("starting server at %s", serverAddr)
	log.Fatal(srv.Start())
}
