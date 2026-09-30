package txmanager

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestJournalRoundTripAndClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "transactions.json")
	if err := os.Mkdir(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	s := mustOpenJournal(t, path)
	data, err := s.load()
	if err != nil || data != nil {
		t.Fatalf("initial load = %q, %v; want nil state", data, err)
	}
	assertJournalPermissions(t, filepath.Dir(path), 0700)
	assertJournalPermissions(t, path+".lock", 0600)
	lockBefore, err := os.Stat(path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{[]byte(`{"nonce":7}`), []byte(`{"nonce":8,"attempts":["example"]}`)} {
		if err := s.save(want); err != nil {
			t.Fatal(err)
		}
		got, err := s.load()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("load = %q, %v; want %q", got, err, want)
		}
		assertJournalPermissions(t, path, 0600)
		matches, err := filepath.Glob(path + ".tmp-*")
		if err != nil || len(matches) != 0 {
			t.Fatalf("temporary files = %v, %v", matches, err)
		}
	}
	if err := s.close(); err != nil {
		t.Fatal(err)
	}
	s = mustOpenJournal(t, path)
	data, err = s.load()
	if err != nil || !bytes.Contains(data, []byte(`"nonce":8`)) {
		t.Fatalf("reopened state = %q, %v", data, err)
	}
	for range 2 {
		if err := s.clear(); err != nil {
			t.Fatal(err)
		}
	}
	data, err = s.load()
	if err != nil || data != nil {
		t.Fatalf("cleared state = %q, %v; want nil", data, err)
	}
	lockAfter, err := os.Stat(path + ".lock")
	if err != nil || !os.SameFile(lockBefore, lockAfter) {
		t.Fatalf("stable lock replaced or removed: %v", err)
	}
}

func TestJournalExclusiveOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transactions.json")
	s := mustOpenJournal(t, path)
	for _, operation := range []string{"open", "save", "clear"} {
		t.Run(operation, func(t *testing.T) {
			switch operation {
			case "save":
				if err := s.save([]byte(`{"nonce":7}`)); err != nil {
					t.Fatal(err)
				}
			case "clear":
				if err := s.clear(); err != nil {
					t.Fatal(err)
				}
			}
			other, err := openJournal(path)
			if err == nil {
				if closeErr := other.close(); closeErr != nil {
					t.Fatal(closeErr)
				}
				t.Fatal("second owner acquired journal lock")
			}
		})
	}
	if err := s.close(); err != nil {
		t.Fatal(err)
	}
	// A failed competing open must close its own descriptor without affecting the owner.
	other := mustOpenJournal(t, path)
	if err := other.close(); err != nil {
		t.Fatal(err)
	}
}

func TestJournalRejectsUnsafeFiles(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "directory", "fifo", "public permissions"} {
		for _, target := range []string{"state", "lock"} {
			t.Run(target+"/"+kind, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "transactions.json")
				unsafePath := path
				if target == "lock" {
					unsafePath += ".lock"
				}
				createUnsafeJournalFile(t, unsafePath, kind)
				if target == "lock" {
					s, err := openJournal(path)
					if err == nil {
						if closeErr := s.close(); closeErr != nil {
							t.Fatal(closeErr)
						}
						t.Fatal("opened unsafe lock")
					}
					return
				}
				s := mustOpenJournal(t, path)
				if _, err := s.load(); err == nil {
					t.Fatal("loaded unsafe state")
				}
				if err := s.save([]byte(`{"nonce":9}`)); err == nil {
					t.Fatal("overwrote unsafe state")
				}
				if err := s.clear(); err == nil {
					t.Fatal("cleared unsafe state")
				}
				if _, err := os.Lstat(unsafePath); err != nil {
					t.Fatalf("unsafe state changed: %v", err)
				}
			})
		}
	}
}

func TestJournalRejectsUnsafeDirectories(t *testing.T) {
	for _, kind := range []string{"symlink", "public writable", "file", "empty path", "missing parent"} {
		t.Run(kind, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "journal")
			path := filepath.Join(dir, "transactions.json")
			switch kind {
			case "symlink":
				if err := os.Symlink(t.TempDir(), dir); err != nil {
					t.Fatal(err)
				}
			case "public writable":
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(dir, 0777); err != nil {
					t.Fatal(err)
				}
			case "file":
				if err := os.WriteFile(dir, []byte("example"), 0600); err != nil {
					t.Fatal(err)
				}
			case "empty path":
				path = ""
			case "missing parent":
				// The parent remains absent: openJournal must not create it.
			}
			s, err := openJournal(path)
			if err == nil {
				if closeErr := s.close(); closeErr != nil {
					t.Fatal(closeErr)
				}
				t.Fatal("opened unsafe directory/path")
			}
			if kind == "missing parent" {
				if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
					t.Fatalf("missing parent was created: %v", statErr)
				}
			}
		})
	}
}

func TestJournalStateSizeGuard(t *testing.T) {
	for _, size := range []int{0, journalMaxBytes + 1} {
		t.Run(stringSizeLabel(size), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "transactions.json")
			s := mustOpenJournal(t, path)
			if err := s.save(make([]byte, size)); err == nil {
				t.Fatal("saved invalid-size state")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("failed save created state: %v", err)
			}
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Truncate(path, int64(size)); err != nil {
				t.Fatal(err)
			}
			if _, err := s.load(); err == nil {
				t.Fatal("loaded invalid-size state")
			}
		})
	}
}

func TestJournalClosedStoreFails(t *testing.T) {
	s := mustOpenJournal(t, filepath.Join(t.TempDir(), "transactions.json"))
	if err := s.close(); err != nil {
		t.Fatal(err)
	}
	if err := s.close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	for _, operation := range []struct {
		name string
		run  func() error
	}{
		{"load", func() error { _, err := s.load(); return err }},
		{"save", func() error { return s.save([]byte(`{"nonce":7}`)) }},
		{"clear", s.clear},
	} {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.run(); err == nil {
				t.Fatal("operation succeeded on closed journal")
			}
		})
	}
}

func mustOpenJournal(t *testing.T, path string) *journalStore {
	t.Helper()
	s, err := openJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func assertJournalPermissions(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}

func createUnsafeJournalFile(t *testing.T, path, kind string) {
	t.Helper()
	switch kind {
	case "symlink", "hardlink":
		target := path + ".target"
		if err := os.WriteFile(target, []byte(`{"nonce":7}`), 0600); err != nil {
			t.Fatal(err)
		}
		var err error
		if kind == "symlink" {
			err = os.Symlink(target, path)
		} else {
			err = os.Link(target, path)
		}
		if err != nil {
			t.Fatal(err)
		}
	case "directory":
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	case "fifo":
		if err := unix.Mkfifo(path, 0600); err != nil {
			t.Fatal(err)
		}
	case "public permissions":
		if err := os.WriteFile(path, []byte(`{"nonce":7}`), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func stringSizeLabel(size int) string {
	if size == 0 {
		return "empty"
	}
	return "oversized"
}
