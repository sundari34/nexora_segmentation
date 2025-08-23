package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/nexora/nexora_segmentation/internal/db"
	"github.com/nexora/nexora_segmentation/internal/models"
	"github.com/nexora/nexora_segmentation/internal/utils"
)

// Evaluate runs the two-layer filter and returns matching members.
func Evaluate(req models.SegmentPayload) ([]models.Member, error) {
	candidates, err := prefilterCandidates(req)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return []models.Member{}, nil
	}

	members, err := deepFilter(req, candidates)
	if err != nil {
		return nil, err
	}

	if req.SegmentID != "" && len(members) > 0 {
		if err := db.SaveSegmentMembers(req.SegmentID, members); err != nil {
			return nil, err
		}
	}

	return members, nil
}

// ---------------------- PREFILTER ----------------------

func prefilterCandidates(req models.SegmentPayload) ([]string, error) {
	if len(req.Groups) == 0 || len(req.Groups[0].Filters) == 0 {
		return nil, fmt.Errorf("no filters provided")
	}
	f := req.Groups[0].Filters[0]

	loc, _ := time.LoadLocation("Asia/Kolkata")
	start, end, err := utils.DeriveDateRange(f.Time.Operator, f.Time.Start, f.Time.End, f.Time.Value, time.Now(), loc)
	if err != nil {
		return nil, err
	}

	op, err := mapCountOperator(f.Count.Operator)
	if err != nil {
		return nil, err
	}
	countVal, err := parseIntStrict(f.Count.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid count.value: %v", err)
	}

	eventCategory := mapEventType(f.EventType)

	var b strings.Builder
	b.WriteString(`
		SELECT nexora_id
		FROM event_daily
		WHERE event_category = ?
		  AND event_date >= toDate(?)
		  AND event_date <= toDate(?)
		GROUP BY nexora_id
		HAVING sum(event_count) `)
	b.WriteString(op)
	b.WriteString(` ?`)

	q := b.String()
	params := []any{eventCategory, start, end, countVal}

	if db.IsQueryLoggingEnabled() {
		log.Printf("[ClickHouse] Query: %s | Params: %+v\n", q, params)
	}

	conn := db.GetClickhouse()
	ctx := context.Background()

	rows, err := conn.Query(ctx, q, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ---------------------- DEEP FILTER ----------------------

func deepFilter(req models.SegmentPayload, nexoraIDs []string) ([]models.Member, error) {
	f := req.Groups[0].Filters[0]

	loc, _ := time.LoadLocation("Asia/Kolkata")
	start, end, err := utils.DeriveDateRange(f.Time.Operator, f.Time.Start, f.Time.End, f.Time.Value, time.Now(), loc)
	if err != nil {
		return nil, err
	}

	eventCategory := mapEventType(f.EventType)

	var whereExtra string
	var args []any
	if f.Query != nil && *f.Query != "" {
		w, a, err := buildWhereFromRQB(*f.Query)
		if err != nil {
			return nil, err
		}
		whereExtra = w
		args = append(args, a...)
	}

	inPh := makePlaceholders(len(nexoraIDs))
	where := `
		WHERE event_category = ?
		  AND event_date >= toDate(?)
		  AND event_date <= toDate(?)
		  AND nexora_id IN (` + inPh + `)
	`
	if whereExtra != "" {
		where += " AND " + whereExtra
	}

	q := `
		SELECT DISTINCT nexora_id, client_id
		FROM events
	` + where

	params := []any{eventCategory, start, end}
	for _, id := range nexoraIDs {
		params = append(params, id)
	}
	params = append(params, args...)

	if db.IsQueryLoggingEnabled() {
		log.Printf("[ClickHouse] Query: %s | Params: %+v\n", q, params)
	}

	conn := db.GetClickhouse()
	ctx := context.Background()

	rows, err := conn.Query(ctx, q, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.Member
	for rows.Next() {
		var m models.Member
		if err := rows.Scan(&m.NexoraID, &m.ClientID); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// ---------------------- Helpers ----------------------

func mapEventType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "system events", "system_event":
		return "system_event"
	case "custom events", "custom_event":
		return "custom_event"
	default:
		return strings.ToLower(strings.ReplaceAll(s, " ", "_"))
	}
}

func mapCountOperator(op string) (string, error) {
	switch op {
	case "equal_to":
		return "=", nil
	case "greater_than":
		return ">", nil
	case "less_than":
		return "<", nil
	case "at_least":
		return ">=", nil
	case "at_most":
		return "<=", nil
	default:
		return "", fmt.Errorf("unsupported count.operator: %s", op)
	}
}

func parseIntStrict(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	return n, err
}

func makePlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}

func getCH() clickhouse.Conn {
	return db.GetClickhouse()
}
