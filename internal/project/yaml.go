package project

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// UpdateChapters returns src with book.chapters replaced by chapters,
// preserving all other content including comments. Missing book or
// chapters keys are created.
func UpdateChapters(src []byte, chapters []string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, err
	}
	if doc.Kind == 0 { // empty document
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{
			{Kind: yaml.MappingNode},
		}}
	}
	root := doc.Content[0]
	book := mapValue(root, "book")
	list := mapValue(book, "chapters")
	list.Kind = yaml.SequenceNode
	list.Tag = "!!seq"
	list.Value = ""
	list.Content = nil
	for _, c := range chapters {
		list.Content = append(list.Content, &yaml.Node{
			Kind: yaml.ScalarNode, Tag: "!!str", Value: c,
		})
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, err
	}
	enc.Close()
	return buf.Bytes(), nil
}

// mapValue returns the value node for key in mapping node m, adding the
// entry if it is missing.
func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	val := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, val)
	return val
}

// hasChapters reports whether the yaml document contains a book.chapters key.
func hasChapters(src []byte) bool {
	var doc struct {
		Book struct {
			Chapters []any `yaml:"chapters"`
		} `yaml:"book"`
	}
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return false
	}
	return doc.Book.Chapters != nil
}

// hasBook reports whether the yaml document contains a top-level book key.
func hasBook(src []byte) bool {
	var doc map[string]any
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return false
	}
	_, ok := doc["book"]
	return ok
}

// flavorMarkers maps a profile flavor suffix to the page marker it selects.
var flavorMarkers = map[string]string{"fw": markerFW, "pol": markerPOL}

// profileTarget splits a profile name into the folder that holds the
// profile's chapters and the set of page markers its flavor suffixes
// select (calltaker-pol → calltaker, POL). An empty marker set means the
// profile has no flavor and takes every page of its folder.
func profileTarget(name string) (dir string, markers map[string]bool) {
	markers = map[string]bool{}
	for trimmed := true; trimmed; {
		trimmed = false
		for flavor, marker := range flavorMarkers {
			suf := "-" + flavor
			if len(name) > len(suf) && strings.EqualFold(name[len(name)-len(suf):], suf) {
				name, trimmed = name[:len(name)-len(suf)], true
				markers[marker] = true
			}
		}
	}
	return name, markers
}

// profileChapters returns the chapter list for the named profile: the
// pages of the profile's folder (including the folder's section page) in
// display order, without pages marked for a different flavor. Unmarked
// pages belong to every flavor.
func (t *Tree) profileChapters(name string) []string {
	dir, markers := profileTarget(name)
	out := []string{}
	var walk func([]*Page)
	walk = func(pages []*Page) {
		for _, p := range pages {
			inDir := p.Path == dir+".qmd" || strings.HasPrefix(p.Path, dir+"/")
			if p.Path != "" && inDir &&
				(len(markers) == 0 || p.Marker == "" || markers[p.Marker]) {
				out = append(out, p.Path)
			}
			walk(p.Children)
		}
	}
	walk(t.Pages)
	return out
}

// WriteChapters writes chapter lists into the book.chapters key of each
// selected profile config (_quarto-<name>.yml) that configures a book, and
// of _quarto.yml if it already maintains a chapter list. _quarto.yml gets
// the full list; each profile gets only the chapters of its own folder,
// named after the profile minus flavor suffixes, filtered to the profile's
// flavor. Selected profiles without a book key are left untouched.
func (t *Tree) WriteChapters(profiles []string) error {
	main := filepath.Join(t.Root, "_quarto.yml")
	if src, err := os.ReadFile(main); err == nil && hasChapters(src) {
		if err := updateChaptersFile(main, src, t.Chapters()); err != nil {
			return err
		}
	}
	for _, p := range profiles {
		file := filepath.Join(t.Root, "_quarto-"+p+".yml")
		src, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if !hasBook(src) {
			continue
		}
		if err := updateChaptersFile(file, src, t.profileChapters(p)); err != nil {
			return err
		}
	}
	return nil
}

func updateChaptersFile(file string, src []byte, chapters []string) error {
	out, err := UpdateChapters(src, chapters)
	if err != nil {
		return err
	}
	return os.WriteFile(file, out, 0o644)
}
