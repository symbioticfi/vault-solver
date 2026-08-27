package architecture_test

import (
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const modulePath = "github.com/symbioticfi/vault-solver/"

type packageInfo struct {
	ImportPath string   `json:"ImportPath"`
	Imports    []string `json:"Imports"`
}

func TestInternalDependencyDirection(t *testing.T) {
	packages := listPackages(t)
	for _, pkg := range packages {
		internalPath := strings.TrimPrefix(pkg.ImportPath, modulePath)
		owner := solverOwner(internalPath)
		for _, dependency := range pkg.Imports {
			dependencyPath := strings.TrimPrefix(dependency, modulePath)
			if owner == "" && strings.HasPrefix(dependencyPath, "internal/solvers/") {
				t.Errorf("neutral package %s imports integration %s", internalPath, dependencyPath)
			}
			dependencyOwner := solverOwner(dependencyPath)
			if owner != "" && dependencyOwner != "" && dependencyOwner != owner {
				t.Errorf("integration %s imports integration %s", internalPath, dependencyPath)
			}
			if isGeneric(internalPath) && isProtocolSpecificAPI(dependencyPath) {
				t.Errorf("generic package %s imports protocol API %s", internalPath, dependencyPath)
			}
		}
	}
}

func listPackages(t *testing.T) []packageInfo {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	cmd := exec.CommandContext(t.Context(), "go", "list", "-json", "./internal/...")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	var packages []packageInfo
	for {
		var pkg packageInfo
		err := decoder.Decode(&pkg)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		packages = append(packages, pkg)
	}
	return packages
}

func solverOwner(path string) string {
	const prefix = "internal/solvers/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	remainder := strings.TrimPrefix(path, prefix)
	owner, _, _ := strings.Cut(remainder, "/")
	return owner
}

func isGeneric(path string) bool {
	for _, prefix := range []string{
		"internal/chain",
		"internal/config",
		"internal/observability",
		"internal/signer",
		"internal/solver",
		"internal/txmanager",
		"internal/version",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func isProtocolSpecificAPI(path string) bool {
	for _, prefix := range []string{
		"api/threef",
		"api/rfqbackend",
		"api/lifiorder",
		"api/uniswapxservice",
		"api/morphographql",
		"api/bindings/3f",
		"api/bindings/rfq",
		"api/bindings/lifi",
		"api/bindings/uniswapx",
		"api/bindings/oev",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
