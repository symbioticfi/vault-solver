package txmanager

import (
	"crypto/rand"
	"io"
	"os"
	"path/filepath"

	"github.com/go-errors/errors"
	"golang.org/x/sys/unix"
)

// journalMaxBytes bounds allocation when reading an untrusted/corrupt journal.
// It is a file-format resource guard, not an operational transaction limit.
const journalMaxBytes = 16 << 20

// journalStore has one lifecycle owner; its methods must not run concurrently.
// The stable lock inode remains present across atomic state-file replacements.
// Directory-relative I/O and O_NOFOLLOW prevent state/lock symlink traversal.
type journalStore struct {
	dir  *os.File
	lock *os.File
	name string
}

// openJournal requires an existing operator-provisioned parent directory on a
// durable volume. Creating it here would require syncing every ancestor entry
// before a saved journal could be trusted to survive a machine crash.
func openJournal(path string) (_ *journalStore, err error) {
	if path == "" || filepath.Base(filepath.Clean(path)) == "." || filepath.Clean(path) == string(filepath.Separator) {
		return nil, errors.New("transaction journal path must name a file")
	}
	dirPath := filepath.Dir(path)
	fd, err := unix.Open(dirPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, errors.Errorf("open transaction journal directory: %w", err)
	}
	s := &journalStore{dir: os.NewFile(uintptr(fd), dirPath), name: filepath.Base(path)}
	defer func() {
		if err != nil {
			err = errors.Join(err, s.close())
		}
	}()
	info, err := s.dir.Stat()
	if err != nil {
		return nil, errors.Errorf("stat transaction journal directory: %w", err)
	}
	if info.Mode().Perm()&0022 != 0 {
		return nil, errors.New("transaction journal directory must not be writable by group or others")
	}
	s.lock, err = s.openFile(s.name+".lock", unix.O_RDWR|unix.O_CREAT)
	if err != nil {
		return nil, errors.Errorf("open transaction journal lock: %w", err)
	}
	if err := unix.Flock(int(s.lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, errors.Errorf("acquire transaction journal lock: %w", err)
	}
	return s, nil
}

func (s *journalStore) load() (_ []byte, err error) {
	f, err := s.openFile(s.name, unix.O_RDONLY)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Errorf("open transaction journal state: %w", err)
	}
	defer func() { err = errors.Join(err, journalCloseFile(f, "state")) }()
	data, err := io.ReadAll(io.LimitReader(f, journalMaxBytes+1))
	if err != nil {
		return nil, errors.Errorf("read transaction journal state: %w", err)
	}
	if len(data) == 0 || len(data) > journalMaxBytes {
		return nil, errors.New("transaction journal state is empty or exceeds the file-size limit")
	}
	return data, nil
}

func (s *journalStore) save(data []byte) (err error) {
	if len(data) == 0 || len(data) > journalMaxBytes {
		return errors.New("transaction journal state is empty or exceeds the file-size limit")
	}
	if err := s.checkState(); err != nil {
		return err
	}
	name := s.name + ".tmp-" + rand.Text()
	f, err := s.openFile(name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL)
	if err != nil {
		return errors.Errorf("create transaction journal temporary file: %w", err)
	}
	defer func() {
		if f != nil {
			err = errors.Join(err, journalCloseFile(f, "temporary file"))
		}
		if cleanupErr := unix.Unlinkat(int(s.dir.Fd()), name, 0); cleanupErr != nil && !errors.Is(cleanupErr, unix.ENOENT) {
			err = errors.Join(err, errors.Errorf("remove transaction journal temporary file: %w", cleanupErr))
		}
	}()
	if _, err := f.Write(data); err != nil {
		return errors.Errorf("write transaction journal state: %w", err)
	}
	if err := f.Sync(); err != nil {
		return errors.Errorf("sync transaction journal state: %w", err)
	}
	err = journalCloseFile(f, "temporary file")
	f = nil
	if err != nil {
		return err
	}
	if err := unix.Renameat(int(s.dir.Fd()), name, int(s.dir.Fd()), s.name); err != nil {
		return errors.Errorf("replace transaction journal state: %w", err)
	}
	if err := s.dir.Sync(); err != nil {
		return errors.Errorf("sync transaction journal directory: %w", err)
	}
	return nil
}

func (s *journalStore) clear() error {
	if err := s.checkState(); err != nil {
		return err
	}
	if err := unix.Unlinkat(int(s.dir.Fd()), s.name, 0); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return errors.Errorf("remove transaction journal state: %w", err)
	}
	if err := s.dir.Sync(); err != nil {
		return errors.Errorf("sync cleared transaction journal directory: %w", err)
	}
	return nil
}

func (s *journalStore) close() error {
	var err error
	if s.lock != nil {
		err = journalCloseFile(s.lock, "lock") // Closing releases flock atomically.
		s.lock = nil
	}
	if s.dir != nil {
		err = errors.Join(err, journalCloseFile(s.dir, "directory"))
		s.dir = nil
	}
	return err
}

func (s *journalStore) checkState() error {
	f, err := s.openFile(s.name, unix.O_RDONLY)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.Errorf("check transaction journal state: %w", err)
	}
	return journalCloseFile(f, "state")
}

func (s *journalStore) openFile(name string, flags int) (_ *os.File, err error) {
	if s.dir == nil {
		return nil, errors.New("transaction journal is closed")
	}
	fd, err := unix.Openat(int(s.dir.Fd()), name, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, errors.Errorf("open journal file %s: %w", name, err)
	}
	f := os.NewFile(uintptr(fd), name)
	defer func() {
		if err != nil {
			err = errors.Join(err, journalCloseFile(f, name))
		}
	}()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, errors.Errorf("stat journal file %s: %w", name, err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return nil, errors.Errorf("journal file %s must be a regular file with one link", name)
	}
	if stat.Mode&0077 != 0 || stat.Uid != uint32(os.Geteuid()) {
		return nil, errors.Errorf("journal file %s must be owned by this user with private permissions", name)
	}
	return f, nil
}

func journalCloseFile(f *os.File, label string) error {
	if err := f.Close(); err != nil {
		return errors.Errorf("close transaction journal %s: %w", label, err)
	}
	return nil
}
