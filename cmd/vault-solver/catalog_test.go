package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
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

type configSchema struct {
	Definitions schemaDefinitions `json:"$defs"`
}

type schemaDefinitions struct {
	SolverEntry schemaSolverEntry `json:"solverEntry"`
}

type schemaSolverEntry struct {
	OneOf []schemaSolverVariant `json:"oneOf"`
}

type schemaSolverVariant struct {
	Properties map[string]schemaProperty `json:"properties"`
}

type schemaProperty struct {
	Const string `json:"const"`
}

func TestSolverCatalogMatchesREADMEAndExamples(t *testing.T) {
	setExampleEnvironment(t)
	registered := solver.Registered()
	root := filepath.Clean(filepath.Join("..", ".."))
	fromExamples := exampleSolverNames(t, root)
	fromREADME := readmeSolverNames(t, filepath.Join(root, "README.md"))
	fromSchema := schemaSolverNames(t, filepath.Join(root, "config", "vault-solver.schema.json"))

	if !slices.Equal(fromExamples, registered) {
		t.Fatalf("example solver names = %v, registered = %v", fromExamples, registered)
	}
	if !slices.Equal(fromREADME, registered) {
		t.Fatalf("README solver names = %v, registered = %v", fromREADME, registered)
	}
	if !slices.Equal(fromSchema, registered) {
		t.Fatalf("schema solver names = %v, registered = %v", fromSchema, registered)
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
		if !strings.Contains(string(data), "# yaml-language-server: $schema=./vault-solver.schema.json") {
			t.Errorf("%s does not declare the committed config schema", path)
		}
		if err := validateConfigFile(path); err != nil {
			t.Errorf("offline validation of %s: %v", path, err)
		}
		data = []byte(os.ExpandEnv(string(data)))
		var catalog exampleCatalog
		if err := yaml.Unmarshal(data, &catalog); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if len(catalog.Solvers) != 1 || catalog.Solvers[0].Name == "" {
			t.Fatalf("%s must contain exactly one named solver", path)
		}
		entry := catalog.Solvers[0]
		if err := solver.ValidateConfig(entry.Name, entry.Config); err != nil {
			t.Errorf("validate %s: %v", path, err)
		}
		names = append(names, entry.Name)
	}
	slices.Sort(names)
	return names
}

func schemaSolverNames(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config schema: %v", err)
	}
	var schema configSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse config schema: %v", err)
	}
	names := make([]string, 0, len(schema.Definitions.SolverEntry.OneOf))
	for _, variant := range schema.Definitions.SolverEntry.OneOf {
		name := variant.Properties["name"].Const
		if name == "" {
			t.Fatal("config schema solver variant has no name const")
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func setExampleEnvironment(t *testing.T) {
	t.Helper()
	values := map[string]string{
		"ETH_RPC_URL_MAINNET":        "https://rpc.example",
		"ETH_RPC_URL_SEPOLIA":        "https://rpc.example",
		"ETH_USD_FEED":               "0x0000000000000000000000000000000000000011",
		"LIQUIDLANE_ADAPTER":         "0x0000000000000000000000000000000000000012",
		"LIFI_EXECUTOR":              "0x0000000000000000000000000000000000000018",
		"OEV_MORPHO_API_URL":         "https://morpho.example/graphql",
		"RFQ_BACKEND_URL":            "https://rfq.example",
		"TLOAN_USD_FEED":             "0x0000000000000000000000000000000000000013",
		"TOKEN_TO_REDEEM":            "0x0000000000000000000000000000000000000014",
		"UNISWAPX_EXECUTOR":          "0x0000000000000000000000000000000000000015",
		"VAULT_ASSET":                "0x0000000000000000000000000000000000000016",
		"VAULT_ASSET_USD_FEED":       "0x0000000000000000000000000000000000000017",
		"WRITE_RPC_URL":              "https://write-rpc.example",
		"ETH_RPC_URL_MAINNET_BACKUP": "https://backup-rpc.example",
		"ETH_RPC_URL_SEPOLIA_BACKUP": "https://backup-rpc.example",
	}
	for name, value := range values {
		t.Setenv(name, value)
	}
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
