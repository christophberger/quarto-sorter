package project

import (
	"bytes"
	"os"
	"path"
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

// hasMake reports whether the yaml document contains a top-level make key.
// The make key (listing output profile names) marks a subproject's
// _quarto-<name>.yml as a flavor profile in multiproject mode.
func hasMake(src []byte) bool {
	var doc map[string]any
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return false
	}
	_, ok := doc["make"]
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

// flavorOf returns the page markers a multiproject flavor profile name
// selects: every dash-separated token naming a known flavor contributes
// its marker (handout-fw → FW, fw-pol → FW and POL). No matching token
// means no flavor: the profile takes every page.
func flavorOf(name string) map[string]bool {
	markers := map[string]bool{}
	for _, tok := range strings.Split(name, "-") {
		for flavor, marker := range flavorMarkers {
			if strings.EqualFold(tok, flavor) {
				markers[marker] = true
			}
		}
	}
	return markers
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

// subChapters returns the chapter list of the subproject in folder sub:
// its pages in display order, with paths relative to the subfolder (each
// subfolder is its own Quarto project), without pages marked for a
// different flavor. Unmarked pages belong to every flavor.
func (t *Tree) subChapters(sub string, markers map[string]bool) []string {
	prefix := sub + "/"
	out := []string{}
	var walk func([]*Page)
	walk = func(pages []*Page) {
		for _, p := range pages {
			if p.Path != "" && strings.HasPrefix(p.Path, prefix) &&
				(len(markers) == 0 || p.Marker == "" || markers[p.Marker]) {
				out = append(out, strings.TrimPrefix(p.Path, prefix))
			}
			walk(p.Children)
		}
	}
	walk(t.Pages)
	return out
}

// WriteChapters writes the chapter lists derived from the tree into the
// project's configs. _quarto.yml gets the full list if it already
// maintains a chapter list.
//
// In a multiproject root (subfolders with their own _quarto.yml), each
// subproject's _quarto.yml that configures a book gets the chapters of
// its folder — every subproject, selected or not, so that a page moved
// across subfolders leaves the source list and enters the target one —
// and each selected flavor profile (<sub>/<name>) gets that list
// filtered to the profile's flavor. Chapter paths are relative to the
// subfolder.
//
// Otherwise, each selected profile config (_quarto-<name>.yml) that
// configures a book gets the chapters of its own folder, named after the
// profile minus flavor suffixes, filtered to the profile's flavor.
// Selected profiles without a book key are left untouched.
func (t *Tree) WriteChapters(profiles []string) error {
	main := filepath.Join(t.Root, "_quarto.yml")
	if src, err := os.ReadFile(main); err == nil && hasChapters(src) {
		if err := updateChaptersFile(main, src, topLevelIndex(t.Chapters())); err != nil {
			return err
		}
	}
	subs, err := Subprojects(t.Root)
	if err != nil {
		return err
	}
	if len(subs) > 0 {
		return t.writeSubprojectChapters(subs, profiles)
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
		if err := updateChaptersFile(file, src, topLevelIndex(t.profileChapters(p))); err != nil {
			return err
		}
	}
	return nil
}

// writeSubprojectChapters syncs the configs of a multiproject root; see
// WriteChapters. Flavor profiles are written even without a book key:
// they are book config fragments layered over the subproject's
// _quarto.yml, so book.chapters is created when missing.
func (t *Tree) writeSubprojectChapters(subs, profiles []string) error {
	for _, sub := range subs {
		file := filepath.Join(t.Root, sub, "_quarto.yml")
		src, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if !hasBook(src) {
			continue
		}
		if err := updateChaptersFile(file, src, topLevelIndex(t.subChapters(sub, nil))); err != nil {
			return err
		}
	}
	for _, p := range profiles {
		sub, name, ok := strings.Cut(p, "/")
		if !ok {
			continue
		}
		file := filepath.Join(t.Root, sub, "_quarto-"+name+".yml")
		src, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if err := updateChaptersFile(file, src, topLevelIndex(t.subChapters(sub, flavorOf(name)))); err != nil {
			return err
		}
	}
	return nil
}

// topLevelIndex rewrites a chapter list whose first entry is a subfolder's
// section page ("<dir>/index.qmd") to start with a bare "index.qmd". Quarto
// requires a book's opening chapter to be a plain index.qmd at the project
// root; books kept in subfolders satisfy this with a root index.qmd that
// includes the folder's real section page. Only the leading entry is
// touched — later "<dir>/index.qmd" subsections are left as they are.
func topLevelIndex(chapters []string) []string {
	if len(chapters) == 0 {
		return chapters
	}
	first := chapters[0]
	if path.Base(first) != "index.qmd" || path.Dir(first) == "." {
		return chapters
	}
	out := make([]string, len(chapters))
	copy(out, chapters)
	out[0] = "index.qmd"
	return out
}

func updateChaptersFile(file string, src []byte, chapters []string) error {
	out, err := UpdateChapters(src, chapters)
	if err != nil {
		return err
	}
	return os.WriteFile(file, out, 0o644)
}
