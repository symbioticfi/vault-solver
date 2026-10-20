package main

import (
	"context"
	"os"
	"time"

	"github.com/go-errors/errors"
	"github.com/spf13/cobra"
	"github.com/symbioticfi/vault-solver/internal/config"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solvers/merkle/threefapi"
)

// Key generation rotates the provider credential; it is an explicit standalone
// operation and must never run as part of the solver's startup or retry loop.
func newThreeFKeyCmd() *cobra.Command {
	var origin, keyEnv, out string
	cmd := &cobra.Command{Use: "threef-key", Short: "Generate/rotate a dedicated 3F BF API key into a new private file", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return generateThreeFKey(cmd.Context(), origin, keyEnv, out)
	}}
	cmd.Flags().StringVar(&origin, "url", "https://bf.3f.xyz", "3F API origin")
	cmd.Flags().StringVar(&keyEnv, "key-env", "", "environment variable holding the dedicated BF EOA key")
	cmd.Flags().StringVar(&out, "out", "", "new output file for the API key (mode 0600)")
	_ = cmd.MarkFlagRequired("key-env")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}
func generateThreeFKey(ctx context.Context, origin, keyEnv, path string) (err error) {
	if keyEnv == "" || path == "" {
		return errors.New("3f key environment name and output path are required")
	}
	sgnr, err := signer.FromConfig(config.SignerConfig{KeyEnv: keyEnv})
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.New("3f output must be a new writable private file")
	}
	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = os.Remove(path)
		}
	}()
	call, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	key, err := threefapi.GenerateKey(call, origin, sgnr)
	if err != nil {
		return err
	}
	if _, err = file.WriteString(key + "\n"); err != nil {
		return errors.New("3f key file write failed")
	}
	if err = file.Sync(); err != nil {
		return errors.New("3f key file synchronization failed")
	}
	complete = true
	return nil
}
