// Quarto Sorter is a local web app for reordering the pages of a Quarto
// project by drag and drop. It keeps the order frontmatter fields and the
// Markdown heading levels in sync with the tree.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

// Set via -ldflags at build time (see .goreleaser.yaml).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	addr := flag.String("addr", "localhost:8199", "listen address")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("quarto-sorter %s (commit %s, built %s)\n", version, commit, date)
		os.Exit(0)
	}

	srv, err := newServer()
	if err != nil {
		log.Fatal(err)
	}
	if dir := flag.Arg(0); dir != "" {
		if err := srv.setRoot(dir); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("Quarto Sorter running on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, srv))
}
