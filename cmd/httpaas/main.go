// Command httpaas is the management web app of the local HTTP-as-a-Service platform.
// It provisions Apache VMs from a multiattach disk, registers them in Bind9 and
// deploys user content. This phase (A0) only serves a health check.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("httpaas", flag.ContinueOnError)
	listen := fs.String("listen", "127.0.0.1:8080", "address to listen on")
	if err := fs.Parse(args); err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              *listen,
		Handler:           newMux(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Printf("escuchando en http://%s\n", *listen)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve http: %w", err)
	}
	return nil
}

// newMux builds the HTTP routes. Kept separate from run so tests can exercise it.
func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /salud", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "ok")
	})
	return mux
}
