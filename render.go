package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/christophberger/quarto-sorter/internal/bookmaker"
)

// buildPrefix and slidesPrefix name the flat documents handed to Quarto.
//
// The leading underscore is what tells Quarto the file is not project
// content: without it a website build renders every flat book a second time
// as a stray top-level page and `contents: auto` lists it in the sidebar. It
// also keeps the file out of the sorter's own page tree. An
// underscore-prefixed file still renders when named on the command line,
// which is exactly how it is used here.
const (
	buildPrefix  = "_book-build-"
	slidesPrefix = "_slides-build-"
)

// slidesFormat is what a deck is rendered to; the book's PDF/DOCX formats
// make no sense for it.
const slidesFormat = "revealjs"

// renderOpts is one render request.
type renderOpts struct {
	// Root is the project root; the flat documents are written there so
	// that website-absolute media paths such as /assets/x.png resolve.
	Root string
	// Books are the book folder names to render.
	Books []string
	// Profiles maps a book folder name to the Quarto profiles it is
	// rendered with — one render run per profile, since the profiles of a
	// book (…-fw, …-pol) select alternative variants of it rather than
	// combining into one document. A book with no profile is rendered once
	// without --profile.
	Profiles map[string][]string
	// Formats are the Quarto output formats for the book, e.g. pdf, docx.
	Formats []string
	// Slides also renders the deck built from the pages' ::: slide blocks.
	Slides bool
}

// job is the single background render the server runs at a time. The tree
// handlers must stay responsive while Quarto works, which can take minutes,
// so the render runs in its own goroutine and the UI polls this for output.
type job struct {
	mu      sync.Mutex
	lines   []string
	running bool
	failed  bool
}

// jobState is a consistent snapshot of a job for rendering.
type jobState struct {
	Lines   []string
	Running bool
	Failed  bool
}

func (j *job) state() jobState {
	j.mu.Lock()
	defer j.mu.Unlock()
	return jobState{
		Lines:   append([]string(nil), j.lines...),
		Running: j.running,
		Failed:  j.failed,
	}
}

func (j *job) logf(format string, args ...any) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.lines = append(j.lines, fmt.Sprintf(format, args...))
}

func (j *job) fail() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.failed = true
}

// start begins a render in the background, reporting whether it was
// accepted: a job already running is left alone.
func (j *job) start(o renderOpts) bool {
	j.mu.Lock()
	if j.running {
		j.mu.Unlock()
		return false
	}
	j.running, j.failed, j.lines = true, false, nil
	j.mu.Unlock()

	go func() {
		j.run(o)
		j.mu.Lock()
		j.running = false
		j.mu.Unlock()
	}()
	return true
}

// run renders every selected book, carrying on after a failure so that one
// broken book does not hide the others.
func (j *job) run(o renderOpts) {
	for _, book := range o.Books {
		if err := j.renderBook(o, book); err != nil {
			j.logf("%s: %v", book, err)
			j.fail()
		}
	}
	if !j.state().Failed {
		j.logf("done: %d book(s) rendered", len(o.Books))
	}
}

// renderBook flattens one book folder and feeds the result to Quarto.
//
// The flat documents are temporary: Quarto has no way to read a document
// from standard input, and it resolves media paths against the file's own
// location, so the book has to exist as a file at the project root for the
// duration of the render and is removed afterwards.
func (j *job) renderBook(o renderOpts, book string) error {
	res, err := flattenBook(o.Root, book)
	if err != nil {
		return err
	}
	for _, w := range res.Warnings {
		j.logf("%s: warning: %s", book, w)
	}
	j.logf("%s: %d pages, %d slides, %d links rewritten",
		book, res.Pages, res.SlideBlocks, res.Links)

	bookFile := filepath.Join(o.Root, buildPrefix+book+".qmd")
	if err := os.WriteFile(bookFile, []byte(res.Content), 0o644); err != nil {
		return err
	}
	defer j.remove(bookFile)

	slidesFile := ""
	switch {
	case !o.Slides:
	case res.Slides == "":
		j.logf("%s: no ::: slide blocks; no deck rendered", book)
	default:
		slidesFile = filepath.Join(o.Root, slidesPrefix+book+".qmd")
		if err := os.WriteFile(slidesFile, []byte(res.Slides), 0o644); err != nil {
			return err
		}
		defer j.remove(slidesFile)
	}

	// No profile selected still means one run: the book renders with the
	// project's default configuration.
	profiles := o.Profiles[book]
	if len(profiles) == 0 {
		profiles = []string{""}
	}
	var failed bool
	for _, profile := range profiles {
		for _, format := range o.Formats {
			if err := j.quarto(o.Root, bookFile, format, profile); err != nil {
				j.logf("%s: %v", book, err)
				failed = true
			}
		}
		if slidesFile != "" {
			if err := j.quarto(o.Root, slidesFile, slidesFormat, profile); err != nil {
				j.logf("%s: %v", book, err)
				failed = true
			}
		}
	}
	if failed {
		return fmt.Errorf("one or more render runs failed")
	}
	return nil
}

// flattenBook turns a book folder into the flat book and slide documents.
func flattenBook(root, book string) (*bookmaker.Result, error) {
	dir := filepath.Join(root, book)
	tree, err := bookmaker.LoadTree(dir, root)
	if err != nil {
		return nil, err
	}
	if tree == nil {
		return nil, fmt.Errorf("%s holds no .qmd files", book)
	}
	return bookmaker.Flatten(tree, bookmaker.Options{
		ProjectRoot:   root,
		RewriteLinks:  true,
		WrapAudience:  true,
		ExtractSlides: true,
	})
}

// quartoCommand is the executable to run; tests replace it.
var quartoCommand = "quarto"

// quarto runs one `quarto render` and streams its output into the job log.
func (j *job) quarto(root, input, format, profile string) error {
	args := []string{"render", filepath.Base(input), "--to", format}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	j.logf("$ %s %s", quartoCommand, strings.Join(args, " "))

	cmd := exec.Command(quartoCommand, args...)
	cmd.Dir = root // profiles and project config are found from here
	w := &lineWriter{job: j}
	cmd.Stdout, cmd.Stderr = w, w

	err := cmd.Run()
	w.flush()
	if err != nil {
		return fmt.Errorf("%s --to %s: %w", filepath.Base(input), format, err)
	}
	return nil
}

// remove deletes a generated build file, reporting a failure into the log
// rather than to the caller: the render itself already succeeded.
func (j *job) remove(file string) {
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		j.logf("could not remove %s: %v", filepath.Base(file), err)
	}
}

// lineWriter feeds a command's output into a job's log line by line.
type lineWriter struct {
	job *job
	buf bytes.Buffer
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// Keep the partial line for the next write.
			w.buf.Reset()
			w.buf.WriteString(line)
			return len(p), nil
		}
		w.job.logf("%s", strings.TrimRight(line, "\r\n"))
	}
}

// flush emits whatever the command left without a trailing newline.
func (w *lineWriter) flush() {
	if rest := strings.TrimRight(w.buf.String(), "\r\n"); rest != "" {
		w.job.logf("%s", rest)
	}
	w.buf.Reset()
}

// books lists the project's book folders by name: the first-level folders
// that hold Quarto content. Media folders, dot/underscore entries, and
// folders with a Quarto project config of their own are not books.
func books(root string) []string {
	dirs, err := bookmaker.BookFolders(root)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(dirs))
	for _, d := range dirs {
		names = append(names, filepath.Base(d))
	}
	return names
}
