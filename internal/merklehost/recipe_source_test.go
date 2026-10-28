package merklehost

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

func TestIndexedRecipeHTTPAuthenticatesOriginalProgram(t *testing.T) {
	executor := common.HexToAddress("0x11")
	recipe := recipes.Recipe{Queries: [][]byte{{1, 2}}, Steps: [][]byte{{3, 4}}}
	hash, err := recipes.HashRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"original", func(map[string]any) {}},
		{"wrong executor", func(p map[string]any) { p["executor"] = common.HexToAddress("0x99").Hex() }},
		{"wrong hash", func(p map[string]any) { p["recipeHash"] = common.HexToHash("0x99").Hex() }},
		{"changed program", func(p map[string]any) { p["steps"] = []string{"0x01"} }},
		{"bad bytes", func(p map[string]any) { p["queries"] = []string{"0xgg"} }},
		{"bad block", func(p map[string]any) { p["blockNumber"] = "-1" }},
		{"later publication of same recipe", func(p map[string]any) { p["blockNumber"] = "8" }},
		{"missing block hash", func(p map[string]any) { delete(p, "blockHash") }},
		{"negative log index", func(p map[string]any) { p["logIndex"] = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/recipes/"+strings.ToLower(executor.Hex()) || r.URL.Query().Get("hash") != hash.Hex() || r.URL.Query().Get("block") != "7" {
					t.Error("indexed lookup escaped executor/hash/publication block")
				}
				p := map[string]any{"executor": executor.Hex(), "recipeHash": hash.Hex(), "blockNumber": "7", "blockHash": common.HexToHash("0x33").Hex(), "transactionHash": common.HexToHash("0x44").Hex(), "logIndex": 4, "queries": []string{"0x0102"}, "steps": []string{"0x0304"}}
				tc.mutate(p)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(p)
			}))
			defer server.Close()
			source, err := NewRecipeSource(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			publication, err := source.RecipePublication(t.Context(), executor, hash, 7)
			if tc.name != "original" {
				if err == nil {
					t.Fatal("invalid indexed publication accepted")
				}
				return
			}
			if err != nil || publication.Executor != executor || publication.RecipeHash != hash || publication.BlockNumber != 7 || publication.LogIndex != 4 {
				t.Fatalf("original publication changed: %+v %v", publication, err)
			}
			if !reflect.DeepEqual(publication.Recipe, recipe) {
				t.Fatal("indexed program did not round trip")
			}
		})
	}
}

func TestIndexedRecipeHTTPDoesNotFallbackWhenUnavailable(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusTemporaryRedirect, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Location", "/redirect")
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = w.Write([]byte(strings.Repeat("x", int(permissions.HostMaxResponseBytes)+1)))
				}
			}))
			defer server.Close()
			source, err := NewRecipeSource(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			_, err = source.RecipePublication(t.Context(), common.HexToAddress("0x11"), common.HexToHash("0x22"), 7)
			if err == nil || requests != 1 || strings.Contains(err.Error(), server.URL) {
				t.Fatalf("unavailable publication retried or exposed source: requests=%d err=%v", requests, err)
			}
		})
	}
}
