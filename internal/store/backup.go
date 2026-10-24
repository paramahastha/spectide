package store

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spectide/spectide/internal/model"
)

const currentSchemaVersion = 14

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

// ImportLegacyWorkspace copies a confirmed legacy database into a new default
// workspace. It snapshots the source first, refuses to overwrite either the
// backup or destination, and retains the source file.
func ImportLegacyWorkspace(ctx context.Context, source, destination string, confirmed bool) (string, error) {
	if !confirmed {
		return "", fmt.Errorf("IMPORT_CONFIRMATION_REQUIRED")
	}
	src, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	dest, err := filepath.Abs(destination)
	if err != nil {
		return "", err
	}
	if src == dest {
		return "", fmt.Errorf("IMPORT_SOURCE_EQUALS_DESTINATION")
	}
	sourceInfo, statErr := os.Lstat(src)
	if statErr != nil || !sourceInfo.Mode().IsRegular() || sourceInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("IMPORT_SOURCE_INVALID")
	}
	if _, err = os.Lstat(dest); err == nil {
		return "", fmt.Errorf("IMPORT_DESTINATION_EXISTS")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return "", fmt.Errorf("IMPORT_DIRECTORY_UNAVAILABLE")
	}
	if err = validateBackup(ctx, src); err != nil {
		return "", err
	}
	backup := dest + ".legacy-" + time.Now().UTC().Format("20060102T150405.000000000") + ".backup"
	db, err := sql.Open("sqlite", src)
	if err != nil {
		return "", fmt.Errorf("IMPORT_SOURCE_UNAVAILABLE")
	}
	db.SetMaxOpenConns(1)
	tmp, err := os.CreateTemp(filepath.Dir(backup), ".spectide-import-backup-*")
	if err != nil {
		_ = db.Close()
		return "", err
	}
	tmpPath := tmp.Name()
	if err = tmp.Close(); err != nil {
		_ = db.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err = os.Remove(tmpPath); err != nil {
		_ = db.Close()
		return "", err
	}
	quoted := strings.ReplaceAll(tmpPath, "'", "''")
	_, err = db.ExecContext(ctx, `VACUUM INTO '`+quoted+`'`)
	closeErr := db.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("IMPORT_BACKUP_FAILED: %w", err)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", closeErr
	}
	if err = os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err = validateBackup(ctx, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err = os.Link(tmpPath, backup); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("IMPORT_BACKUP_INSTALL_FAILED: %w", err)
	}
	_ = os.Remove(tmpPath)
	if err = RestoreBackup(ctx, backup, dest); err != nil {
		return backup, err
	}
	destinationStore, err := Open(dest)
	if err != nil {
		return backup, err
	}
	defer destinationStore.Close()
	destinationID, err := destinationStore.WorkspaceID(ctx)
	if err != nil {
		return backup, err
	}
	sourceID := "legacy-" + uuid.NewSHA1(uuid.NameSpaceURL, []byte(src)).String()
	nowText := time.Now().UTC().Format(time.RFC3339Nano)
	importID := uuid.NewString()
	tx, err := destinationStore.DB.BeginTx(ctx, nil)
	if err != nil {
		return backup, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO workspace_imports(id,source_workspace_id,destination_workspace_id,status,backup_reference,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, importID, sourceID, destinationID, "COMPLETED", backup, nowText, nowText); err != nil {
		return backup, fmt.Errorf("IMPORT_RECORD_FAILED: %w", err)
	}
	for sequence, status := range []string{"DETECTED", "AWAITING_CONFIRMATION", "BACKED_UP", "RESTORED", "VALIDATED", "COMPLETED"} {
		if _, err = tx.ExecContext(ctx, `INSERT INTO workspace_import_events(id,import_id,sequence,status,occurred_at) VALUES(?,?,?,?,?)`, uuid.NewString(), importID, sequence+1, status, nowText); err != nil {
			return backup, fmt.Errorf("IMPORT_EVENT_RECORD_FAILED: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return backup, fmt.Errorf("IMPORT_RECORD_FAILED: %w", err)
	}
	return backup, nil
}

func (s *Store) WorkspaceImports(ctx context.Context) ([]model.WorkspaceImport, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,source_workspace_id,destination_workspace_id,status,backup_reference,created_at,updated_at FROM workspace_imports ORDER BY created_at DESC,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.WorkspaceImport{}
	for rows.Next() {
		var item model.WorkspaceImport
		var created, updated string
		if err = rows.Scan(&item.ID, &item.SourceWorkspaceID, &item.DestinationWorkspaceID, &item.Status, &item.BackupReference, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) WorkspaceImportEvents(ctx context.Context, importID string) ([]model.WorkspaceImportEvent, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,import_id,sequence,status,occurred_at FROM workspace_import_events WHERE import_id=? ORDER BY sequence`, importID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.WorkspaceImportEvent{}
	for rows.Next() {
		var item model.WorkspaceImportEvent
		var occurred string
		if err = rows.Scan(&item.ID, &item.ImportID, &item.Sequence, &item.Status, &occurred); err != nil {
			return nil, err
		}
		item.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurred)
		items = append(items, item)
	}
	return items, rows.Err()
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
