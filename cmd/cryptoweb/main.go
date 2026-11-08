// cryptoweb displays persisted optimization history without market access.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/marcgauthier/Crypto/internal/dashboard"
	"github.com/marcgauthier/Crypto/internal/findings"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "local HTTP listen address")
	sqlitePath := flag.String("db", "./data/findings.sqlite", "SQLite optimization history database")
	flag.Parse()
	db, err := findings.Open(*sqlitePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open findings:", err)
		os.Exit(1)
	}
	defer db.Close()
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "listen:", err)
		os.Exit(1)
	}
	defer listener.Close()
	log.Printf("Strategy dashboard: http://%s", listener.Addr())
	server := &http.Server{Handler: dashboard.New(db), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
