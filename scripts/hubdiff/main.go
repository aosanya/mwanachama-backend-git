package main

import (
	"bytes"
	"fmt"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: hubdiff <dir>")
		os.Exit(2)
	}
	err := filepath.Walk(os.Args[1], func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		return normalise(path)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func normalise(path string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	file.Comments = nil
	file.Doc = nil
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), file); err != nil {
		return err
	}
	out, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}
