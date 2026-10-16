package merkle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/state"
	sdktx "github.com/symbioticfi/merkle-executor/sdk/submission"
)

func TestSimulationCannotHideRecoveredSignerOwnership(t *testing.T) {
	if err := validateRecoveredMode("simulate", "signed-order"); err == nil {
		t.Fatal("simulation hid unresolved shared signer work")
	}
	if err := validateRecoveredMode("send", "signed-order"); err != nil {
		t.Fatal(err)
	}
	if err := validateRecoveredMode("simulate", ""); err != nil {
		t.Fatal(err)
	}
}

type legacyIdentitySender struct{ pending string }

func (s *legacyIdentitySender) PendingID() string { return s.pending }
func (*legacyIdentitySender) Send(context.Context, sdktx.Request) (sdktx.Result, error) {
	panic("recovery must not send")
}
func (*legacyIdentitySender) Acknowledge(context.Context, string) error {
	panic("recovery must not acknowledge")
}

func TestAddressOnlyHostFencesLegacyOwners(t *testing.T) {
	cfg, err := parseConfig(configNode(t, validConfig))
	if err != nil {
		t.Fatal(err)
	}
	executor := cfg.strategies[0].Config.Executor
	strategies := cfg.strategies
	for _, legacy := range []string{"old-custom", "executor-" + executor} {
		for _, tc := range []struct {
			phase   string
			pending bool
		}{{"prepared", false}, {"authorized", true}, {"settled", true}, {"failed", true}} {
			t.Run(legacy+"/"+tc.phase, func(t *testing.T) {
				directory := t.TempDir()
				store, err := state.Open(directory)
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				job := keeper.Job{ID: "legacy-owned", Phase: tc.phase, ConfigHash: "old-label-policy", Plan: keeper.Plan{Outputs: []provider.Output{{Data: json.RawMessage(`{"uid":"durable-order"}`)}}}}
				key := "strategy." + legacy + ".v1"
				if err := store.Save("strategies.v1", []string{legacy}); err != nil {
					t.Fatal(err)
				}
				if err := store.Save(key, job); err != nil {
					t.Fatal(err)
				}
				sender := &legacyIdentitySender{}
				if tc.pending {
					sender.pending = job.ID
					if err := store.Save("txmanager.active.v1", map[string]string{"id": job.ID, "signed": "existing-signed-bytes"}); err != nil {
						t.Fatal(err)
					}
				}
				bytesFor := func(key string) []byte {
					t.Helper()
					hash := sha256.Sum256([]byte(key))
					raw, err := os.ReadFile(filepath.Join(directory, hex.EncodeToString(hash[:])+".json"))
					if err != nil {
						t.Fatal(err)
					}
					return raw
				}
				before := bytesFor(key)
				var signed []byte
				if tc.pending {
					signed = bytesFor("txmanager.active.v1")
				}
				worker := keeper.Keeper{Store: store, Sender: sender, Strategies: strategies}
				if err := worker.CheckRecovery(); err == nil || !strings.Contains(err.Error(), "removed from configuration") {
					t.Fatalf("legacy owner admitted: %v", err)
				}
				if !bytes.Equal(before, bytesFor(key)) || tc.pending && (!bytes.Equal(signed, bytesFor("txmanager.active.v1")) || sender.pending != job.ID) {
					t.Fatal("recovery rewrote legacy ownership")
				}
				var inventory []string
				if _, err := store.Load("strategies.v1", &inventory); err != nil || !slices.Contains(inventory, legacy) {
					t.Fatal("legacy inventory identifier removed", err)
				}
			})
		}
	}
}
