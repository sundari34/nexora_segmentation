package db

import (
	"database/sql"
	"time"
)

// SaveSegmentResult inserts or updates a segment evaluation result
func SaveSegmentResult(userID string, segmentID string, result bool) error {
	query := `
		INSERT INTO segment_results (user_id, segment_id, result, evaluated_at)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			result = VALUES(result),
			evaluated_at = VALUES(evaluated_at)
	`

	_, err := GetMySQL().Exec(query, userID, segmentID, result, time.Now())
	return err
}

// GetSegmentResult fetches a segment result for a given user and segment
func GetSegmentResult(userID string, segmentID string) (bool, error) {
	query := `
		SELECT result
		FROM segment_results
		WHERE user_id = ? AND segment_id = ?
	`

	var result bool
	err := GetMySQL().QueryRow(query, userID, segmentID).Scan(&result)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return result, err
}
