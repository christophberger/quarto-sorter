package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateChaptersReplaces(t *testing.T) {
	src := []byte(`project:
  type: book
  output-dir: _book
book:
  title: Test Book # keep
  chapters:
    - index.qmd
    - old.qmd
format:
  html: default
`)
	got, err := UpdateChapters(src, []string{"index.qmd", "a.qmd", "b/index.qmd"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{
		"type: book", "output-dir: _book", "title: Test Book # keep",
		"- index.qmd", "- a.qmd", "- b/index.qmd", "html: default",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "old.qmd") {
		t.Errorf("output still contains old.qmd:\n%s", s)
	}
}

func TestUpdateChaptersCreatesBook(t *testing.T) {
	got, err := UpdateChapters([]byte("format:\n  pdf: default\n"), []string{"index.qmd"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, "book:") || !strings.Contains(s, "- index.qmd") || !strings.Contains(s, "pdf: default") {
		t.Errorf("unexpected output:\n%s", s)
	}
}

func TestUpdateChaptersEmptyDoc(t *testing.T) {
	got, err := UpdateChapters(nil, []string{"index.qmd"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "- index.qmd") {
		t.Errorf("unexpected output:\n%s", got)
	}
}

// WriteChapters updates _quarto.yml (which has book.chapters) and the
// selected profiles only.
func TestWriteChapters(t *testing.T) {
	root := writeFixture(t)
	tree, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.WriteChapters([]string{"print"}); err != nil {
		t.Fatal(err)
	}

	main, _ := os.ReadFile(filepath.Join(root, "_quarto.yml"))
	if !strings.Contains(string(main), "- chapter3/deep/leaf.qmd") {
		t.Errorf("_quarto.yml not updated:\n%s", main)
	}
	if !strings.Contains(string(main), "title: Test Book # keep") {
		t.Errorf("_quarto.yml lost content:\n%s", main)
	}

	print_, _ := os.ReadFile(filepath.Join(root, "_quarto-print.yml"))
	if !strings.Contains(string(print_), "- chapter2/second.qmd") {
		t.Errorf("_quarto-print.yml not updated:\n%s", print_)
	}

	web, _ := os.ReadFile(filepath.Join(root, "_quarto-web.yml"))
	if strings.Contains(string(web), "chapters") {
		t.Errorf("_quarto-web.yml should be untouched (not selected):\n%s", web)
	}
}

// _quarto.yml without a book section (e.g. a website project) is left alone.
func TestWriteChaptersSkipsNonBookMain(t *testing.T) {
	root := writeFixture(t)
	orig := "project:\n  type: website\n"
	os.WriteFile(filepath.Join(root, "_quarto.yml"), []byte(orig), 0o644)
	tree, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.WriteChapters(nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "_quarto.yml"))
	if string(got) != orig {
		t.Errorf("_quarto.yml changed:\n%s", got)
	}
}
