package hack

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveImageDigest(t *testing.T) {
	const image = "docker.io/symbioticfi/vault-solver:commit"
	const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		name    string
		stdout  string
		stderr  string
		exit    string
		want    string
		wantErr bool
	}{
		{name: "published image", stdout: digest + "\n", exit: "0", want: "digest=" + digest + "\n"},
		{name: "missing tag", stderr: "ERROR: " + image + ": not found\n", exit: "1", want: "digest=\n"},
		{name: "authentication failure", stderr: "ERROR: unauthorized: authentication required\n", exit: "1", wantErr: true},
		{name: "rate limited", stderr: "ERROR: 429 Too Many Requests\n", exit: "1", wantErr: true},
		{name: "network failure", stderr: "ERROR: registry host not found\n", exit: "1", wantErr: true},
		{name: "mixed failure", stderr: "ERROR: " + image + ": not found\nERROR: connection failed\n", exit: "1", wantErr: true},
		{name: "invalid digest", stdout: "sha256:invalid\n", exit: "0", wantErr: true},
		{name: "empty digest", exit: "0", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			fakeDocker := `#!/usr/bin/env bash
printf '%s\n' "$@" > "$DOCKER_ARGS"
printf '%s' "$DOCKER_STDOUT"
printf '%s' "$DOCKER_STDERR" >&2
exit "$DOCKER_EXIT"
`
			if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(fakeDocker), 0o700); err != nil {
				t.Fatal(err)
			}
			argsFile := filepath.Join(dir, "args")
			cmd := exec.CommandContext(t.Context(), "bash", "resolve-image-digest.sh", image)
			cmd.Env = append(os.Environ(),
				"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"DOCKER_ARGS="+argsFile,
				"DOCKER_STDOUT="+tc.stdout,
				"DOCKER_STDERR="+tc.stderr,
				"DOCKER_EXIT="+tc.exit,
			)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			if (err != nil) != tc.wantErr {
				t.Fatalf("resolve image: %v, stderr: %s", err, &stderr)
			}
			if got := stdout.String(); got != tc.want {
				t.Fatalf("output = %q, want %q", got, tc.want)
			}
			if tc.wantErr && stderr.Len() == 0 {
				t.Fatal("image lookup failure had no diagnostic")
			}
			args, readErr := os.ReadFile(argsFile)
			if readErr != nil {
				t.Fatal(readErr)
			}
			wantArgs := "buildx\nimagetools\ninspect\n" + image + "\n--format\n{{.Manifest.Digest}}\n"
			if string(args) != wantArgs {
				t.Fatalf("docker args = %q, want %q", args, wantArgs)
			}
			if tc.wantErr && tc.exit != "0" && !strings.Contains(stderr.String(), strings.TrimSpace(tc.stderr)) {
				t.Fatalf("registry error was lost: %s", &stderr)
			}
		})
	}
}
