package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solver"
)

var solverTableRow = regexp.MustCompile(`^\| \x60([^\x60]+)\x60 \|`)

type exampleSolverEntry struct {
	Name   string    `yaml:"name"`
	Config yaml.Node `yaml:"config"`
}

type exampleCatalog struct {
	Solvers []exampleSolverEntry `yaml:"solvers"`
}

func TestSolverCatalogMatchesREADMEAndExamples(t *testing.T) {
	registered := solver.Registered()
	root := filepath.Clean(filepath.Join("..", ".."))
	fromExamples := exampleSolverNames(t, root)
	fromREADME := readmeSolverNames(t, filepath.Join(root, "README.md"))

	if !slices.Equal(fromExamples, registered) {
		t.Fatalf("example solver names = %v, registered = %v", fromExamples, registered)
	}
	if !slices.Equal(fromREADME, registered) {
		t.Fatalf("README solver names = %v, registered = %v", fromREADME, registered)
	}
}

func exampleSolverNames(t *testing.T, root string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "config", "*.example.yaml"))
	if err != nil {
		t.Fatalf("glob example configs: %v", err)
	}
	var names []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var catalog exampleCatalog
		if err := yaml.Unmarshal(data, &catalog); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if len(catalog.Solvers) != 1 || catalog.Solvers[0].Name == "" {
			t.Fatalf("%s must contain exactly one named solver", path)
		}
		names = append(names, catalog.Solvers[0].Name)
	}
	slices.Sort(names)
	return names
}

func readmeSolverNames(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open README: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close README: %v", err)
		}
	}()

	var names []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		match := solverTableRow.FindStringSubmatch(scanner.Text())
		if len(match) == 2 && match[1] != "solver.name" {
			names = append(names, match[1])
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan README: %v", err)
	}
	slices.Sort(names)
	return names
}
