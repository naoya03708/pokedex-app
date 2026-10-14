package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRender_SectionsAppearInOrderWithTheirTitleAndText(t *testing.T) {
	got := render([]section{
		{Title: "Pokedex", Text: "MIT text\n"},
		{Title: "example.com/dep v1.2.3", Text: "BSD text\n"},
	})

	for _, want := range []string{"Pokedex", "MIT text", "example.com/dep v1.2.3", "BSD text"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "MIT text") > strings.Index(got, "BSD text") {
		t.Error("sections are not in the given order")
	}
	if !strings.HasSuffix(got, "\n") {
		t.Error("output does not end with a newline")
	}
}

func TestCollect_ReadsLicenseFileOfEachModule(t *testing.T) {
	dir := t.TempDir()
	write := func(sub, name, text string) string {
		d := filepath.Join(dir, sub)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return d
	}
	a := write("a", "LICENSE", "license of a")
	b := write("b", "LICENSE.md", "license of b")
	c := write("c", "COPYING", "license of c")

	got, err := collect([]module{
		{Path: "ex.com/a", Version: "v1.0.0", Dir: a},
		{Path: "ex.com/b", Version: "v2.0.0", Dir: b},
		{Path: "ex.com/c", Version: "v3.0.0", Dir: c},
	})

	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Title != "ex.com/a v1.0.0" || got[0].Text != "license of a" ||
		got[1].Text != "license of b" || got[2].Text != "license of c" {
		t.Fatalf("got %+v", got)
	}
}

func TestCollect_ModuleWithoutLicenseFileIsAnError(t *testing.T) {
	_, err := collect([]module{{Path: "ex.com/nolicense", Version: "v1.0.0", Dir: t.TempDir()}})

	if err == nil || !strings.Contains(err.Error(), "ex.com/nolicense") {
		t.Fatalf("err = %v, want one naming the module", err)
	}
}

func TestGenerate_IncludesOwnLicenseAndTheDependenciesOfTheApp(t *testing.T) {
	got, err := generate("../../..")

	if err != nil {
		t.Fatal(err)
	}
	own, err := os.ReadFile("../../../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, strings.TrimSpace(string(own))) {
		t.Error("the project's own LICENSE is missing")
	}
	for _, want := range []string{"github.com/egoist/mygo ", "github.com/ebitengine/purego ", "golang.org/x/image "} {
		if !strings.Contains(got, want) {
			t.Errorf("no section for %s", want)
		}
	}
	// The Go runtime and standard library are linked into the binary.
	if !strings.Contains(got, "Go standard library and runtime") || !strings.Contains(got, "The Go Authors") {
		t.Error("the Go license is missing")
	}
	// Build-time tools are not part of the app.
	if strings.Contains(got, "go.yaml.in/yaml") {
		t.Error("lists a module that is not compiled into the app")
	}
}

func TestCommittedNoticesAreUpToDate(t *testing.T) {
	want, err := generate("../../..")
	if err != nil {
		t.Fatal(err)
	}
	have, err := os.ReadFile("../../../" + outFile)
	if err != nil {
		t.Fatal(err)
	}

	if string(have) != want {
		t.Errorf("%s is out of date; run: go run ./internal/tools/notices", outFile)
	}
}
