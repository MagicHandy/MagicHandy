package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Contract declarations live in types.go plus the linked participant/status
// types. Both aliased constructors and direct concrete type references count.
func TestMotionUsesOnlyTransportContract(t *testing.T) {
	root := repoRoot(t)
	files := token.NewFileSet()
	contract, err := parser.ParseFile(files, filepath.Join(root, "internal", "transport", "types.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"LinkedParticipant": true, "LinkedSessionStatus": true, "LinkedSessionStatusProvider": true}
	for name := range contract.Scope.Objects {
		allowed[name] = true
	}
	err = filepath.WalkDir(filepath.Join(root, "internal", "motion"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(files, path, nil, 0)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			name, _ := strconv.Unquote(imp.Path.Value)
			if name != modulePath+"/internal/transport" {
				continue
			}
			alias := "transport"
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			if alias == "." {
				t.Errorf("%s uses an unqualified transport import", path)
				continue
			}
			ast.Inspect(file, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := selector.X.(*ast.Ident)
				if ok && pkg.Name == alias && pkg.Obj == nil && !allowed[selector.Sel.Name] {
					t.Errorf("%s uses concrete transport symbol %s", files.Position(node.Pos()), selector.Sel)
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
