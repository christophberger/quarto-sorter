package project

import (
	"bytes"
	"os"
	"path/filepath"

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

// WriteChapters writes the tree's chapter list into the book.chapters key
// of each selected profile config (_quarto-<name>.yml), and of _quarto.yml
// if it already maintains a chapter list.
func (t *Tree) WriteChapters(profiles []string) error {
	chapters := t.Chapters()
	main := filepath.Join(t.Root, "_quarto.yml")
	if src, err := os.ReadFile(main); err == nil && hasChapters(src) {
		if err := updateChaptersFile(main, src, chapters); err != nil {
			return err
		}
	}
	for _, p := range profiles {
		file := filepath.Join(t.Root, "_quarto-"+p+".yml")
		src, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if err := updateChaptersFile(file, src, chapters); err != nil {
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
