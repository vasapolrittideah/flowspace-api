package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCheckReportsPackageLevelErrorsOutsideErrorsFile(t *testing.T) {
	root := t.TempDir()
	write(t, root, "services/identity/internal/app/service.go", `package app

import (
	"errors"
	"fmt"
)

/*
var ErrInComment = errors.New("comment")
*/

var ErrSingle = errors.New("single")

var ErrFirst, ErrSecond = errors.New("first"), errors.New("second")

var (
	ErrGrouped        = errors.New("grouped")
	errFormatted error = fmt.Errorf("formatted: %w", ErrSingle)
	limit              = 5
)

type (
	LimitError struct{}
	Settings   struct{}
)

const example = `+"`"+`
var ErrInString = errors.New("string")
`+"`"+`

func run() error {
	err := errors.New("local")
	return err
}
`)
	write(t, root, "services/identity/internal/app/limit_error.go", `package app

func (LimitError) Error() string { return "limit" }
`)
	write(t, root, "services/identity/internal/app/pointer_error.go", `package app

type PointerError struct{}

func (e *PointerError) Error() string { return "pointer" }
`)
	write(t, root, "services/identity/internal/app/generic_error.go", `package app

type Failure[T any] struct{ Value T }

func (Failure[T]) Error() string { return "failure" }
`)
	write(t, root, "services/identity/internal/app/other_methods.go", `package app

type WithArgument struct{}

func (WithArgument) Error(code int) string { return "argument" }

type WithInt struct{}

func (WithInt) Error() int { return 1 }

type WithTwoResults struct{}

func (WithTwoResults) Error() (string, error) { return "", nil }
`)
	write(t, root, "services/identity/internal/app/errors.go", `package app

import "errors"

var ErrAllowed = errors.New("allowed")

type AllowedError struct{}

func (*AllowedError) Error() string { return "allowed" }
`)
	write(t, root, "internal/authn/authn.go", `package authn

import "errors"

var ErrUnavailable = errors.New("unavailable")
`)
	write(t, root, "internal/authn/authn_test.go", `package authn

import "errors"

var errFake = errors.New("fake")
`)
	write(t, root, "services/identity/internal/adapter/out/postgres/sqlc/db.go", `package sqlc

import "errors"

var ErrGenerated = errors.New("generated")
`)
	write(t, root, "services/identity/cmd/api/main.go", `package main

import "errors"

var errOutsideInternal = errors.New("outside")
`)

	got, err := check(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []finding{
		{Path: "internal/authn/authn.go", Line: 5, Name: "ErrUnavailable"},
		{Path: "services/identity/internal/app/generic_error.go", Line: 3, Name: "Failure"},
		{Path: "services/identity/internal/app/pointer_error.go", Line: 3, Name: "PointerError"},
		{Path: "services/identity/internal/app/service.go", Line: 12, Name: "ErrSingle"},
		{Path: "services/identity/internal/app/service.go", Line: 14, Name: "ErrFirst"},
		{Path: "services/identity/internal/app/service.go", Line: 14, Name: "ErrSecond"},
		{Path: "services/identity/internal/app/service.go", Line: 17, Name: "ErrGrouped"},
		{Path: "services/identity/internal/app/service.go", Line: 18, Name: "errFormatted"},
		{Path: "services/identity/internal/app/service.go", Line: 23, Name: "LimitError"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findings =\n%v\nwant\n%v", got, want)
	}
}

func TestRunReportsFindingsAndExitCode(t *testing.T) {
	root := t.TempDir()
	write(t, root, "internal/authn/authn.go", "package authn\n\nimport \"errors\"\n\nvar ErrUnavailable = errors.New(\"unavailable\")\n")
	var stdout, stderr bytes.Buffer

	if code := run([]string{root}, &stdout, &stderr); code != 1 || stderr.String() != "internal/authn/authn.go:5: Declare package-level error ErrUnavailable in errors.go.\n" {
		t.Fatalf("run = %d, stderr %q", code, stderr.String())
	}

	write(t, root, "internal/authn/authn.go", "package authn\n")
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{root}, &stdout, &stderr); code != 0 || stdout.String() != "Package-level errors are in errors.go files.\n" {
		t.Fatalf("run = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}

	write(t, root, "internal/authn/authn.go", "package authn\n\nvar =\n")
	stderr.Reset()
	if code := run([]string{root}, &stdout, &stderr); code != 1 || stderr.Len() == 0 {
		t.Fatalf("run = %d for a file that does not parse, stderr %q", code, stderr.String())
	}
}

func TestCheckRejectsARootThatIsNotADirectory(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "file.go")
	if err := os.WriteFile(file, []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{file, filepath.Join(directory, "missing")} {
		if _, err := check(root); err == nil || !strings.Contains(err.Error(), root) {
			t.Fatalf("check(%q) error = %v, want an error that names the root", root, err)
		}
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{file}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), file) {
		t.Fatalf("run = %d, stderr %q", code, stderr.String())
	}
}

func write(t *testing.T, root, path, source string) {
	t.Helper()
	file := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}
