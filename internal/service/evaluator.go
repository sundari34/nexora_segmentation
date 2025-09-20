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

// ---- helpers: canonical view of time & count ----
type normalizedTime struct {
	Operator string
	Start    *utils.FlexibleString
	End      *utils.FlexibleString
	Value    *utils.FlexibleString
	DayValue *utils.FlexibleString
	DayCount *utils.FlexibleString
}

type normalizedCount struct {
	Operator string
	Value    *utils.FlexibleString
	Min      *utils.FlexibleString
	Max      *utils.FlexibleString
}

// normalizeFilter extracts normalized time & count from either old or new schema
func normalizeFilter(f models.Filter) (normalizedTime, normalizedCount, int, string, string) {
	var nt normalizedTime
	var nc normalizedCount
	eventID := f.EventID
	cond := f.Condition
	eventName := f.EventName
	eventType := f.EventType

	if f.ConditionBlock != nil {
		log.Printf("[DEBUG] Using ConditionBlock, Time operator: %q, Count operator: %q", f.ConditionBlock.Time.Operator, f.ConditionBlock.Count.Operator)
		// --- TIME ---
		nt.Operator = strings.ToLower(strings.TrimSpace(f.ConditionBlock.Time.Operator))
		nt.Value = f.ConditionBlock.Time.Value
		nt.Start = f.ConditionBlock.Time.StartDate
		nt.End = f.ConditionBlock.Time.EndDate

		// --- COUNT ---
		nc.Operator = strings.ToLower(strings.TrimSpace(f.ConditionBlock.Count.Operator))
		nc.Value = f.ConditionBlock.Count.Value
		nc.Min = f.ConditionBlock.Count.Min
		nc.Max = f.ConditionBlock.Count.Max

		eventID = f.ConditionBlock.EventID
		cond = f.ConditionBlock.Condition
		eventName = f.ConditionBlock.EventName
		eventType = f.ConditionBlock.EventType
	} else {
		log.Printf("[DEBUG] Using old flattened filter, Time operator: %q, Count operator: %q", f.Time.Operator, f.Count.Operator)
		nt.Operator = strings.ToLower(strings.TrimSpace(f.Time.Operator))
		nt.Start = f.Time.Start
		nt.End = f.Time.End
		nt.Value = f.Time.Value
		nt.DayValue = f.Time.DayValue
		nt.DayCount = f.Time.DayCountValue

		nc.Operator = strings.ToLower(strings.TrimSpace(f.Count.Operator))
		nc.Value = &f.Count.Value
		nc.Min = f.Count.Min
		nc.Max = f.Count.Max
	}

	return nt, nc, eventID, cond, eventName + "|" + eventType
}

// ---------------------- PREFILTER ----------------------
func prefilterCandidates(req models.SegmentPayload) ([]string, error) {
	if len(req.Groups) == 0 || len(req.Groups[0].Filters) == 0 {
		return nil, fmt.Errorf("no filters provided")
	}

	for gi, group := range req.Groups {
		for fi, f := range group.Filters {
			nt, nc, _, _, _ := normalizeFilter(f)
			log.Printf("[DEBUG] Group %d, Filter %d, Time operator: %q, Count operator: %q", gi, fi, nt.Operator, nc.Operator)

			loc, _ := time.LoadLocation("Asia/Kolkata")
			_, _, err := utils.DeriveDateRange(nt.Operator, nt.Start, nt.End, nt.Value, nt.DayValue, nt.DayCount, time.Now(), loc)
			if err != nil {
				return nil, fmt.Errorf("DeriveDateRange failed at group %d filter %d: %w", gi, fi, err)
			}

			op := nc.Operator
			mapped, err := mapCountOperator(op)
			if err != nil {
				return nil, fmt.Errorf("mapCountOperator failed at group %d filter %d: %w", gi, fi, err)
			}
			log.Printf("[DEBUG] Mapped count operator: %s", mapped)
		}
	}

	f := req.Groups[0].Filters[0]
	nt, nc, _, _, _ := normalizeFilter(f)

	loc, _ := time.LoadLocation("Asia/Kolkata")
	start, end, err := utils.DeriveDateRange(nt.Operator, nt.Start, nt.End, nt.Value, nt.DayValue, nt.DayCount, time.Now(), loc)
	if err != nil {
		return nil, err
	}

	op := nc.Operator
	eventCategory := mapEventType(f.EventType)
	eventName := f.EventName
	if f.ConditionBlock != nil {
		eventCategory = mapEventType(f.ConditionBlock.EventType)
		eventName = f.ConditionBlock.EventName
	}

	timeClause := "event_date >= toDate(?) AND event_date <= toDate(?)"
	timeParams := []any{start, end}
	if strings.ToLower(nt.Operator) == "before" {
		single := nt.Value
		if single == nil {
			single = nt.DayValue
		}
		timeClause = "event_date < toDate(?)"
		timeParams = []any{single}
	}

	var havingClause string
	var params []any

	switch strings.ToLower(op) {
	case "between":
		if nc.Min == nil || nc.Max == nil {
			return nil, fmt.Errorf("between operator requires min and max")
		}
		havingClause = "HAVING sum(event_count) >= ? AND sum(event_count) <= ?"
		params = append(params, eventCategory, eventName)
		params = append(params, timeParams...)
		params = append(params, nc.Min.String(), nc.Max.String())
	default:
		mapped, err := mapCountOperator(op)
		if err != nil {
			return nil, err
		}
		valFS := nc.Value
		if valFS == nil {
			return nil, fmt.Errorf("count value is required")
		}
		countValInt, err := valFS.ToInt()
		if err != nil {
			return nil, fmt.Errorf("invalid count.value: %v", err)
		}
		havingClause = "HAVING sum(event_count) " + mapped + " ?"
		params = append(params, eventCategory, eventName)
		params = append(params, timeParams...)
		params = append(params, countValInt)
	}

	q := fmt.Sprintf(`
		SELECT nexora_id
		FROM event_daily
		WHERE event_category = ?
		  AND event_name = ?
		  AND %s
		  GROUP BY nexora_id
		%s
	`, timeClause, havingClause)

	if db.IsQueryLoggingEnabled() {
		log.Printf("[ClickHouse] Prefilter Query: %s | Params: %+v\n", q, params)
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
	if len(req.Groups) == 0 || len(req.Groups[0].Filters) == 0 {
		return nil, fmt.Errorf("no filters provided")
	}

	for gi, group := range req.Groups {
		for fi, f := range group.Filters {
			nt, _, _, condStr, _ := normalizeFilter(f)
			log.Printf("[DEBUG] DeepFilter Group %d, Filter %d, Time operator: %q, Condition: %q", gi, fi, nt.Operator, condStr)

			loc, _ := time.LoadLocation("Asia/Kolkata")
			_, _, err := utils.DeriveDateRange(nt.Operator, nt.Start, nt.End, nt.Value, nt.DayValue, nt.DayCount, time.Now(), loc)
			if err != nil {
				return nil, fmt.Errorf("DeepFilter DeriveDateRange failed at group %d filter %d: %w", gi, fi, err)
			}
		}
	}

	f := req.Groups[0].Filters[0]
	nt, _, _, condStr, _ := normalizeFilter(f)
	log.Printf("condition string: %s", condStr)

	loc, _ := time.LoadLocation("Asia/Kolkata")
	start, end, err := utils.DeriveDateRange(nt.Operator, nt.Start, nt.End, nt.Value, nt.DayValue, nt.DayCount, time.Now(), loc)
	if err != nil {
		return nil, err
	}

	eventCategory := mapEventType(f.EventType)
	eventName := f.EventName
	if f.ConditionBlock != nil {
		eventCategory = mapEventType(f.ConditionBlock.EventType)
		eventName = f.ConditionBlock.EventName
	}

	timeClause := "event_date >= toDate(?) AND event_date <= toDate(?)"
	timeParams := []any{start, end}
	if strings.ToLower(nt.Operator) == "before" {
		single := nt.Value
		if single == nil {
			single = nt.DayValue
		}
		timeClause = "event_date < toDate(?)"
		timeParams = []any{single}
	}

	inPh := makePlaceholders(len(nexoraIDs))
	baseWhere := fmt.Sprintf(`
		WHERE event_category = ?
		  AND event_name = ?
		  AND %s
		  AND nexora_id IN (%s)
	`, timeClause, inPh)

	var q string
	cond := f.Condition
	if f.ConditionBlock != nil {
		cond = f.ConditionBlock.Condition
	}

	if cond == "has_performed" {
		q = `SELECT DISTINCT nexora_id, client_id FROM events` + baseWhere
	} else if cond == "has_not_performed" {
		q = fmt.Sprintf(`
			SELECT DISTINCT nexora_id, client_id
			FROM customer_profiles
			WHERE nexora_id IN (%s)
			  AND nexora_id NOT IN (
				SELECT nexora_id FROM events %s
			  )
		`, inPh, baseWhere)
	} else {
		return nil, fmt.Errorf("unsupported condition: %s", cond)
	}

	params := []any{eventCategory, eventName}
	params = append(params, timeParams...)
	for _, id := range nexoraIDs {
		params = append(params, id)
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

// ---------------------- MAIN EVALUATE ----------------------
func Evaluate(req models.SegmentPayload) ([]models.Member, error) {
	// 1️⃣ Prefilter candidates based on first filter
	nexoraIDs, err := prefilterCandidates(req)
	if err != nil {
		return nil, err
	}
	if len(nexoraIDs) == 0 {
		return []models.Member{}, nil
	}

	// 2️⃣ Apply deep filter
	return deepFilter(req, nexoraIDs)
}

// ---------------------- HELPERS ----------------------
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
	switch strings.ToLower(op) {
	case "equal_to", "equal":
		return "=", nil
	case "greater_than", "greater":
		return ">", nil
	case "less_than", "less":
		return "<", nil
	case "at_least":
		return ">=", nil
	case "at_most":
		return "<=", nil
	case "between":
		return "between", nil
	default:
		return "", fmt.Errorf("unsupported count.operator: %s", op)
	}
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
