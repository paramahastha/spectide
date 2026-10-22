package store

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const currentSchemaVersion = 5

// Backup writes a transactionally consistent SQLite snapshot with private file
// permissions. The destination must not already exist.
func (s *Store) Backup(ctx context.Context, destination string) error {
	dest, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if dest == s.path {
		return fmt.Errorf("BACKUP_DESTINATION_IS_DATABASE")
	}
	if _, err = os.Stat(filepath.Dir(dest)); err != nil {
		return fmt.Errorf("BACKUP_DIRECTORY_UNAVAILABLE: %w", err)
	}
	if _, err = os.Lstat(dest); err == nil {
		return fmt.Errorf("BACKUP_DESTINATION_EXISTS")
	} else if !os.IsNotExist(err) {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(dest), ".spectide-backup-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	if err = temp.Close(); err != nil {
		os.Remove(tempPath)
		return err
	}
	if err = os.Remove(tempPath); err != nil {
		return err
	}
	quoted := strings.ReplaceAll(tempPath, "'", "''")
	if _, err = s.DB.ExecContext(ctx, `VACUUM INTO '`+quoted+`'`); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("BACKUP_FAILED: %w", err)
	}
	if err = os.Chmod(tempPath, 0o600); err != nil {
		os.Remove(tempPath)
		return err
	}
	if err = os.Link(tempPath, dest); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("BACKUP_DESTINATION_UNAVAILABLE: %w", err)
	}
	return os.Remove(tempPath)
}

// RestoreBackup validates integrity and schema version before atomically
// installing a private copy at a new destination, then runs supported forward
// migrations. It never overwrites an existing database.
func RestoreBackup(ctx context.Context, source, destination string) error {
	src, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	dest, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if src == dest {
		return fmt.Errorf("RESTORE_SOURCE_EQUALS_DESTINATION")
	}
	sourceInfo, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("RESTORE_SOURCE_UNAVAILABLE: %w", err)
	}
	if !sourceInfo.Mode().IsRegular() || sourceInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("RESTORE_SOURCE_NOT_REGULAR_FILE")
	}
	if err = validateBackup(ctx, src); err != nil {
		return err
	}
	if _, err = os.Stat(filepath.Dir(dest)); err != nil {
		return fmt.Errorf("RESTORE_DIRECTORY_UNAVAILABLE: %w", err)
	}
	if _, err = os.Lstat(dest); err == nil {
		return fmt.Errorf("RESTORE_DESTINATION_EXISTS")
	} else if !os.IsNotExist(err) {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(dest), ".spectide-restore-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	in, err := os.Open(src)
	if err != nil {
		temp.Close()
		return err
	}
	_, copyErr := io.Copy(temp, in)
	closeInErr, closeOutErr := in.Close(), temp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeInErr != nil {
		return closeInErr
	}
	if closeOutErr != nil {
		return closeOutErr
	}
	if err = os.Chmod(tempPath, 0o600); err != nil {
		return err
	}
	if err = os.Link(tempPath, dest); err != nil {
		return fmt.Errorf("RESTORE_DESTINATION_UNAVAILABLE: %w", err)
	}
	validated, err := Open(dest)
	if err != nil {
		os.Remove(dest)
		return fmt.Errorf("RESTORE_MIGRATION_FAILED: %w", err)
	}
	return validated.Close()
}

func validateBackup(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("RESTORE_BACKUP_INVALID: %w", err)
	}
	defer db.Close()
	var integrity string
	if err = db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return fmt.Errorf("RESTORE_INTEGRITY_CHECK_FAILED: %s", integrity)
	}
	var version int
	if err = db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&version); err != nil {
		return fmt.Errorf("RESTORE_SCHEMA_UNSUPPORTED: %w", err)
	}
	if version < 1 || version > currentSchemaVersion {
		return fmt.Errorf("RESTORE_SCHEMA_UNSUPPORTED: version %d (supported through %d)", version, currentSchemaVersion)
	}
	return nil
}
