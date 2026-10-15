package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestAddUsesGitignore(t *testing.T) {
	root, err := ioutil.TempDir("", "sloc-gitignore-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	writeFile := func(name, contents string) {
		if err := ioutil.WriteFile(filepath.Join(root, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := ioutil.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.go\nignored-dir/\nnested/ignored.go\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "ignored-dir"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(root, "nested", ".gitignore"), []byte("*.go\n!keep.go\n"), 0600); err != nil {
		t.Fatal(err)
	}
	writeFile("keep.go", "package keep\n")
	writeFile("ignored.go", "package ignored\n")
	writeFile(filepath.Join("nested", "keep.go"), "package keep\n")
	writeFile(filepath.Join("nested", "drop.go"), "package drop\n")
	writeFile(filepath.Join("nested", "ignored.go"), "package ignored\n")
	writeFile(filepath.Join("ignored-dir", "drop.go"), "package drop\n")

	oldFiles, oldIgnoreDirs := files, ignoreDirs
	defer func() {
		files = oldFiles
		ignoreDirs = oldIgnoreDirs
	}()
	files = nil
	ignoreDirs = nil
	add(root)

	got := make(map[string]bool)
	for _, name := range files {
		got[filepath.Clean(name)] = true
	}
	want := map[string]bool{
		filepath.Join(root, "keep.go"):           true,
		filepath.Join(root, "nested", "keep.go"): true,
	}
	if len(got) != len(want) {
		t.Fatalf("got files %v, want %v", got, want)
	}
	for name := range want {
		if !got[name] {
			t.Errorf("file %s was not selected; got %v", name, got)
		}
	}
}

func TestYAMLLanguage(t *testing.T) {
	for _, name := range []string{"config.yaml", "config.yml", "path/to/config.yml"} {
		var l language
		ok := false
		for _, lang := range languages {
			if lang.Match(name) {
				ok = true
				l = lang
				break
			}
		}
		if !ok {
			t.Fatalf("%s was not matched by any language", name)
		}
		if l.Name() != "YAML" {
			t.Fatalf("%s matched language %s, want YAML", name, l.Name())
		}
	}

	s := &stats{}
	l := language{"YAML", mExt(".yaml", ".yml"), shComments}
	l.Update([]byte("# comment\nkey: value\n\n  # indented\nother: 1\n"), s)
	if s.TotalLines != 5 {
		t.Fatalf("TotalLines = %d, want 5", s.TotalLines)
	}
	if s.CodeLines != 2 {
		t.Fatalf("CodeLines = %d, want 2", s.CodeLines)
	}
	if s.CommentLines != 2 {
		t.Fatalf("CommentLines = %d, want 2", s.CommentLines)
	}
	if s.BlankLines != 1 {
		t.Fatalf("BlankLines = %d, want 1", s.BlankLines)
	}
}
