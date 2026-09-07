package handler

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// nib's own documentation is shipped inside the binary so the agent can read it,
// and it is deliberately not reachable over HTTP: no route serves it, and
// nothing in this package may import the package that holds it. A file server or
// a docs endpoint added later has to be a conscious decision, not a side effect,
// so this fails the build instead.
//
// The other half of the guarantee is tested in skill_test.go: the skills API
// reads the data volume only, so the system skills that carry the documentation
// are a 404 there.
func TestHandlersDoNotServeNibDocumentation(t *testing.T) {
	t.Parallel()

	pkgs, err := parser.ParseDir(token.NewFileSet(), ".", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse handler package: %v", err)
	}

	forbidden := []string{
		"github.com/javdet/nib/internal/nibdocs",
	}

	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			for _, imp := range file.Imports {
				path, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					t.Fatalf("%s: unquote import %s: %v", name, imp.Path.Value, err)
				}
				for _, banned := range forbidden {
					if path == banned {
						t.Fatalf("%s imports %s; nib's documentation must not be reachable over HTTP", name, banned)
					}
				}
			}
		}
	}
}
