// Command check-error-files reports package-level errors that a package declares outside its errors.go file.
package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
)

type finding struct {
	Path string
	Line int
	Name string
}

type goFile struct {
	path string
	file *ast.File
}

var checkedPath = regexp.MustCompile(`^(internal|services/[^/]+/internal)/`)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run checks the folder in args, or the current folder, and returns the exit code.
func run(args []string, stdout, stderr io.Writer) int {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}
	findings, err := check(root)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	for _, f := range findings {
		_, _ = fmt.Fprintf(stderr, "%s:%d: Declare package-level error %s in errors.go.\n", f.Path, f.Line, f.Name)
	}
	if len(findings) > 0 {
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "Package-level errors are in errors.go files.")
	return 0
}

func check(root string) ([]finding, error) {
	files := os.DirFS(root)
	info, err := fs.Stat(files, ".")
	if err != nil {
		return nil, fmt.Errorf("check %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("check %s: not a directory", root)
	}
	fset := token.NewFileSet()
	packages, err := parsePackages(fset, files)
	if err != nil {
		return nil, fmt.Errorf("check %s: %w", root, err)
	}

	var findings []finding
	for _, files := range packages {
		errorTypes := errorTypeNames(files)
		for _, f := range files {
			if path.Base(f.path) == "errors.go" {
				continue
			}
			for _, name := range errorDeclarations(f.file, errorTypes) {
				findings = append(findings, finding{Path: f.path, Line: fset.Position(name.Pos()).Line, Name: name.Name})
			}
		}
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}

// parsePackages parses the handwritten production Go files that the errors.go rule covers, grouped by package directory.
func parsePackages(fset *token.FileSet, files fs.FS) (map[string][]goFile, error) {
	packages := map[string][]goFile{}
	for _, dir := range []string{"internal", "services"} {
		err := fs.WalkDir(files, dir, func(file string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == "sqlc" {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") || !checkedPath.MatchString(file) {
				return nil
			}
			source, err := fs.ReadFile(files, file)
			if err != nil {
				return err
			}
			parsed, err := parser.ParseFile(fset, file, source, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			packages[path.Dir(file)] = append(packages[path.Dir(file)], goFile{path: file, file: parsed})
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return packages, nil
}

// errorTypeNames returns the types of a package that have an Error() string method.
func errorTypeNames(files []goFile) map[string]bool {
	names := map[string]bool{}
	for _, f := range files {
		for _, decl := range f.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "Error" || fn.Type.Params.NumFields() != 0 || fn.Type.Results.NumFields() != 1 {
				continue
			}
			if result, ok := fn.Type.Results.List[0].Type.(*ast.Ident); !ok || result.Name != "string" {
				continue
			}
			receiver := fn.Recv.List[0].Type
			if star, ok := receiver.(*ast.StarExpr); ok {
				receiver = star.X
			}
			switch generic := receiver.(type) {
			case *ast.IndexExpr:
				receiver = generic.X
			case *ast.IndexListExpr:
				receiver = generic.X
			}
			if ident, ok := receiver.(*ast.Ident); ok {
				names[ident.Name] = true
			}
		}
	}
	return names
}

// errorDeclarations returns the package-level error variables and error types of a file.
func errorDeclarations(file *ast.File, errorTypes map[string]bool) []*ast.Ident {
	var names []*ast.Ident
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			switch spec := spec.(type) {
			case *ast.TypeSpec:
				if errorTypes[spec.Name.Name] {
					names = append(names, spec.Name)
				}
			case *ast.ValueSpec:
				if len(spec.Values) != len(spec.Names) {
					continue
				}
				for i, name := range spec.Names {
					if isErrorConstructor(spec.Values[i]) {
						names = append(names, name)
					}
				}
			}
		}
	}
	return names
}

func isErrorConstructor(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && (pkg.Name == "errors" && selector.Sel.Name == "New" || pkg.Name == "fmt" && selector.Sel.Name == "Errorf")
}
