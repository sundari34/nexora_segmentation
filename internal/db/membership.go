package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/nexora/nexora_segmentation/internal/models"
)

// DDL suggestion (execute once):
// CREATE TABLE IF NOT EXISTS segment_members (
//   id BIGINT AUTO_INCREMENT PRIMARY KEY,
//   segment_id VARCHAR(64) NOT NULL,
//   nexora_id VARCHAR(128) NOT NULL,
//   client_id VARCHAR(128) DEFAULT '' NOT NULL,
//   created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
//   updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
//   UNIQUE KEY uniq_segment_member (segment_id, nexora_id)
// );

func SaveSegmentMembers(segmentID string, members []models.Member) error {
	if segmentID == "" || len(members) == 0 {
		return nil
	}
	tx, err := GetMySQL().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
	INSERT INTO segment_members (segment_id, nexora_id, client_id, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE client_id=VALUES(client_id), updated_at=VALUES(updated_at)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for _, m := range members {
		if _, err := stmt.Exec(segmentID, m.NexoraID, m.ClientID, now, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Optional util to bulk delete old members for a segment before re-insert (idempotent refresh).
func ClearSegmentMembers(segmentID string) error {
	if segmentID == "" {
		return nil
	}
	_, err := GetMySQL().Exec("DELETE FROM segment_members WHERE segment_id = ?", segmentID)
	return err
}

// For quick reads if needed
func ListSegmentMembers(segmentID string) ([]models.Member, error) {
	rows, err := GetMySQL().Query("SELECT nexora_id, client_id FROM segment_members WHERE segment_id = ?", segmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Member
	for rows.Next() {
		var m models.Member
		if err := rows.Scan(&m.NexoraID, &m.ClientID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Small helper to convert slice to placeholders (?, ?, ...)
func makePlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}

func toAnySlice(in []string) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}

// Not exported but kept here for package-local reuse
func Errf(format string, a ...any) error {
	return fmt.Errorf(format, a...)
}
