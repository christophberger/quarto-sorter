// Quarto Sorter is a local web app for reordering the pages of a Quarto
// project by drag and drop. It keeps the order frontmatter fields, the
// Markdown heading levels, and the book.chapters lists of the project and
// profile configs in sync.
package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", "localhost:8199", "listen address")
	flag.Parse()

	srv, err := newServer(defaultPrefsFile())
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
