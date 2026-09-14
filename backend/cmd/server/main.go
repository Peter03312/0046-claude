// Command storyproof-api serves the StoryProof backend API.
package main

import (
	"log"
	"net/http"
	"os"

	"storyproof/api"
	"storyproof/store"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	path := os.Getenv("DATABASE_DSN")
	if path == "" {
		dir := os.Getenv("DB_DIR")
		if dir == "" {
			dir = "/data"
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("data dir: %v", err)
		}
		path = dir + "/storyproof.db"
	}
	// SQLite pragmas for a small single-node service.
	st, err := store.Open(path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer st.Close()

	srv := api.NewServer(st)
	log.Printf("storyproof api listening on %s (db %s)", addr, path)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
