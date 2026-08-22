package cache

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

const (
	driverName    = "sqlite"
	engineVersion = 1
	busyTimeoutMS = 5000
)

// migrations[i] upgrades a DB from version i+1 to i+2.
// Version 1 is the baseline in schema.sql; the slice is empty until v2.
var migrations = []string{}

// ErrNotFound means no matching cache row exists.
var ErrNotFound = errors.New("cache: not found")

// DB is a local SQLite cache of API messages and profiles.
type DB struct {
	sql  *sql.DB
	path string
}

var memSeq atomic.Uint64

// Open creates or migrates a file-backed cache at path.
func Open(path string) (*DB, error) {
	if path == "" {
		return nil, errors.New("cache path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	c, err := openDSN(fileDSN(path), path)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	return c, nil
}

// OpenMemory returns an isolated in-memory cache (tests).
func OpenMemory() (*DB, error) {
	name := fmt.Sprintf("memdb_%d", memSeq.Add(1))
	return openDSN(memoryDSN(name), "")
}

func openDSN(dsn, path string) (*DB, error) {
	sqlDB, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	if err = sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	c := &DB{sql: sqlDB, path: path}
	if err = c.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return c, nil
}

func fileDSN(path string) string {
	return "file:" + filepath.ToSlash(path) + "?" + pragmaQuery()
}

func memoryDSN(name string) string {
	return "file:" + name + "?mode=memory&cache=shared&" + pragmaQuery()
}

func pragmaQuery() string {
	return fmt.Sprintf(
		"_pragma=busy_timeout(%d)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)",
		busyTimeoutMS,
	)
}

func (d *DB) migrate() error {
	ver, ok, err := d.currentVersion()
	if err != nil {
		return err
	}
	if !ok {
		if _, err = d.sql.Exec(schemaSQL); err != nil {
			return fmt.Errorf("create schema: %w", err)
		}
		return nil
	}
	if ver > engineVersion {
		return fmt.Errorf("cache is version %d, remiterm supports %d", ver, engineVersion)
	}
	for ver < engineVersion {
		idx := ver - 1
		if idx < 0 || idx >= len(migrations) {
			return fmt.Errorf("no migration from version %d to %d", ver, ver+1)
		}
		if _, err = d.sql.Exec(migrations[idx]); err != nil {
			return fmt.Errorf("migrate to %d: %w", ver+1, err)
		}
		if _, err = d.sql.Exec(`update db_version set version = ?`, ver+1); err != nil {
			return fmt.Errorf("bump db_version to %d: %w", ver+1, err)
		}
		ver++
	}
	return nil
}

func (d *DB) currentVersion() (int, bool, error) {
	var ver int
	err := d.sql.QueryRow(`select version from db_version`).Scan(&ver)
	if err == nil {
		return ver, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	// Missing table → fresh file.
	if strings.Contains(err.Error(), "no such table") {
		return 0, false, nil
	}
	return 0, false, fmt.Errorf("read db_version: %w", err)
}

// Close releases the database.
func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}
