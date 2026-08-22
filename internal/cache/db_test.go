package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCreateAndReopen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "chat.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ver, ok, err := db.currentVersion()
	if err != nil || !ok || ver != engineVersion {
		t.Fatalf("version %d ok=%v err=%v", ver, ok, err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("perm %o", st.Mode().Perm())
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ver, ok, err = db.currentVersion()
	if err != nil || !ok || ver != engineVersion {
		t.Fatalf("reopen version %d ok=%v err=%v", ver, ok, err)
	}
}

func TestOpenMemoryIsolated(t *testing.T) {
	t.Parallel()
	a, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	b, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	if a.path != "" || b.path != "" {
		t.Fatalf("memory path %q %q", a.path, b.path)
	}
}

func TestOpenEmptyPath(t *testing.T) {
	t.Parallel()
	if _, err := Open(""); err == nil {
		t.Fatal("expected error")
	}
}
