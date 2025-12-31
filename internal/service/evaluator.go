package service

import (
	"context"
	"encoding/json"
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

// ---------------------- MAIN EVALUATE ----------------------
func Evaluate(req models.SegmentPayload) ([]models.Member, error) {
	// 1️⃣ Prefilter candidates based on first filter
	log.Printf("here")
	log.Printf("%v", req)
	fmt.Println(len(req.Groups))
	fmt.Println("(((((len(req.Groups))))))")

	var nexoraIDs []string
	var err error
	if len(req.Groups) == 0 {
		nexoraIDs = []string{req.NexoraID}
	} else {
		fmt.Println("-----------------------")
		nexoraIDs, err = prefilterCandidates(req)

		if err != nil {
			log.Printf("Error in processing the payload, some conditions in the properties are not handled : %+v", err)
			return nil, err
		}

		if len(nexoraIDs) == 0 {
			return []models.Member{}, nil
		}
	}
	fmt.Println(nexoraIDs)
	fmt.Println("(((((nexoraIDs)))))")
	// ✅ Check if this is a user_property-only segment
	isUserPropertyOnly := true
	for _, group := range req.Groups {
		for _, f := range group.Filters {
			if f.FilterCategory != "user_property" {
				isUserPropertyOnly = false
				break
			}
		}
		if !isUserPropertyOnly {
			break
		}
	}

	// ✅ If only user_property filters, no deep filtering needed
	fmt.Println(isUserPropertyOnly)
	fmt.Println("((((isUserPropertyOnly))))")
	if isUserPropertyOnly {
		clientDBManager := db.NewClientDB()
		mysqlTenantConn, err := clientDBManager.GetMysqlDB(req.ClientID, req.ProjectID)
		if err != nil {
			log.Println(nexoraIDs)
			fmt.Println(err)
			fmt.Println("(((((((((err inside evalute tenant mysql)))))))))")
		}
		var members []models.Member
		log.Println(len(nexoraIDs))
		log.Println(nexoraIDs)
		log.Println("(((((((((((((nexoraIDs)))))))))))))")
		for _, id := range nexoraIDs {

			userPropertySql := ""
			where := ""

			if req.Property != "" {
				userPropertySql = fmt.Sprintf(
					"COALESCE(JSON_UNQUOTE(JSON_EXTRACT(user_properties, '$.%s')), 'default') AS property",
					req.Property,
				)
			} else {
				userPropertySql = "'' AS property"
			}

			if req.Channel != "" {
				if req.Channel == "email" {
					where += " AND email IS NOT NULL AND email != ''"
				} else if req.Channel == "mobile" || req.Channel == "sms" {
					where += " AND mobile IS NOT NULL AND mobile != ''"
				} else if req.Channel == "push" || req.Channel == "web_push" {
					where += " AND id IN (SELECT external_user_id FROM notification_tokens WHERE token IS NOT NULL AND token != '')"
				}
			}

			q := fmt.Sprintf(`
        SELECT %s
        FROM customer_profiles
        WHERE id IN (
            SELECT customer_profile_id
            FROM nexora_profiles
            WHERE nexora_id = '%s'
        ) %s
        LIMIT 1
    `, userPropertySql, id, where)
			fmt.Println(q)
			fmt.Println("((((q))))")
			rows, err := mysqlTenantConn.Query(q)
			if err != nil {
				fmt.Println(err)
				fmt.Println("(((((((((((((((err inside property getting)))))))))))))))")
				return nil, err
			}
			fmt.Println(rows)
			fmt.Println("(((((rows)))))")
			propertyValue := ""
			count := 0
			if rows.Next() {
				count++
				if err := rows.Scan(&propertyValue); err != nil {
					rows.Close()
					return nil, err
				}
			}
			rows.Close() // Don't defer inside loop
			fmt.Println(count)
			fmt.Println("(((((count)))))")
			if count > 0 {
				members = append(members, models.Member{
					NexoraID: id,
					Property: propertyValue,
				})
			}
		}
		fmt.Println(members)
		fmt.Println("(((members)))")
		return members, nil
	}

	// 2️⃣ Apply deep filter
	return deepFilter(req, nexoraIDs)
}

// ---------------------- PREFILTER ----------------------
func prefilterCandidates(req models.SegmentPayload) ([]string, error) {
	if len(req.Groups) == 0 {
		return nil, fmt.Errorf("no filters provided")
	}

	loc, _ := time.LoadLocation("Asia/Kolkata")
	var whereClauses []string
	var havingClauses []string
	var params []any
	var havingParams []any
	log.Printf("before loop")
	fmt.Println(req)
	fmt.Println("((((((((((((((((((((((((((((((((((req))))))))))))))))))))))----------------)))))))))))))")
	clientDBManager := db.NewClientDB()
	mysqlConn, err := clientDBManager.GetMysqlDB(req.ClientID, req.ProjectID)
	clickhouseConn, err := clientDBManager.GetCHDB(req.ClientID, req.ProjectID)
	fmt.Println(err)
	fmt.Println("((((((((((((((err ------------------------------ err))))))))))))))")
	for gi, group := range req.Groups {
		log.Printf("entered loop")
		log.Printf(" Group %v", group)
		if len(group.Filters) == 0 {
			log.Printf("no group")
			continue
		}

		var groupWhere []string
		var groupHaving []string
		var groupParams []any
		var groupHavingParams []any
		var userPropertyIDs []string

		for fi, f := range group.Filters {
			log.Printf("in a loop")
			nt, nc, _, _, _ := normalizeFilter(f)

			b, err := json.MarshalIndent(f, "", "  ")
			if err != nil {
				fmt.Println("error marshalling:", err)

			}
			fmt.Println(string(b))

			if f.ConditionBlock != nil && f.ConditionBlock.UserPropertyQuery != nil {
				upq := f.ConditionBlock.UserPropertyQuery

				var upqLite utils.UserPropertyQueryLite
				b, _ := json.Marshal(upq)
				_ = json.Unmarshal(b, &upqLite)

				whereClause, params, err := utils.BuildUserPropertyQuery(&upqLite)
				if err != nil {
					return nil, fmt.Errorf("error building user property query: %v", err)
				}

				// add nexora_id in condition
				if req.NexoraID != "" {
					whereClause += fmt.Sprintf(" and nexora_id = %s", req.NexoraID)
				}

				if mysqlConn == nil {
					return nil, fmt.Errorf("mysql connection not initialized")
				}

				query := fmt.Sprintf(`
					SELECT np.nexora_id
					FROM nexora_profiles np
					JOIN customer_profiles cp ON np.customer_profile_id = cp.id
					WHERE %s
				`, whereClause)

				// Debug: print query and parameters
				fmt.Println("---- User Property Query ----")
				fmt.Println("Query:", query)
				fmt.Println("Params:", params)
				fmt.Println("-----------------------------")

				rows, err := mysqlConn.Query(query, params...)
				if err != nil {
					return nil, fmt.Errorf("error executing user property query: %v", err)
				}
				defer rows.Close()

				var ids []string
				for rows.Next() {
					var id string
					if err := rows.Scan(&id); err == nil {
						ids = append(ids, id)
					}
				}

				if len(ids) > 0 {
					userPropertyIDs = append(userPropertyIDs, ids...)
				}

				continue
			}

			start, end, err := utils.DeriveDateRange(nt.Operator, nt.Start, nt.End, nt.Value, nt.DayValue, nt.DayCount, time.Now(), loc)
			if err != nil {
				return nil, fmt.Errorf("DeriveDateRange failed at group %d filter %d: %v", gi, fi, err)
			}

			eventCategory := mapEventType(f.EventType)
			eventName := f.EventName
			if f.ConditionBlock != nil {
				eventCategory = mapEventType(f.ConditionBlock.EventType)
				eventName = f.ConditionBlock.EventName
			}

			// WHERE: row-level
			var timeClause string
			var timeParams []any

			op := strings.ToLower(nt.Operator)
			switch op {
			case "before":
				single := nt.Value
				if single == nil {
					single = nt.DayValue
				}
				timeClause = "event_date < toDate(?)"
				timeParams = []any{single}

			case "on":
				single := nt.Value
				if single == nil {
					single = nt.DayValue
				}
				// ✅ For strict equality (date = given date)
				timeClause = "event_date = toDate(?)"
				timeParams = []any{single}

			default:
				timeClause = "event_date >= toDate(?) AND event_date <= toDate(?)"
				timeParams = []any{start, end}
			}

			whereClause := "(event_category = ? AND event_name = ? AND " + timeClause + ")"
			groupWhere = append(groupWhere, whereClause)
			groupParams = append(groupParams, eventCategory, eventName)
			groupParams = append(groupParams, timeParams...)

			// HAVING: aggregate-level
			var havingClause string
			switch strings.ToLower(nc.Operator) {
			case "between":
				if nc.Min == nil || nc.Max == nil {
					return nil, fmt.Errorf("between operator requires min and max")
				}
				havingClause = "sum(event_count) >= ? AND sum(event_count) <= ?"
				groupHavingParams = append(groupHavingParams, nc.Min.String(), nc.Max.String())
			default:
				mapped, err := mapCountOperator(nc.Operator)
				if err != nil {
					return nil, err
				}
				if nc.Value == nil {
					return nil, fmt.Errorf("count value required")
				}
				valInt, err := nc.Value.ToInt()
				if err != nil {
					return nil, fmt.Errorf("invalid count.value: %v", err)
				}
				havingClause = fmt.Sprintf("sum(event_count) %s ?", mapped)
				groupHavingParams = append(groupHavingParams, valInt)
			}
			groupHaving = append(groupHaving, havingClause)
		}

		if len(userPropertyIDs) > 0 {
			placeholder := strings.Repeat("?,", len(userPropertyIDs))
			placeholder = strings.TrimSuffix(placeholder, ",")
			groupWhere = append(groupWhere, fmt.Sprintf("nexora_id IN (%s)", placeholder))
			for _, id := range userPropertyIDs {
				groupParams = append(groupParams, id)
			}
		}
		// combine filters within group
		whereClauses = append(whereClauses, "("+strings.Join(groupWhere, " "+strings.ToUpper(group.MatchMode)+" ")+")")
		if len(groupHaving) > 0 {
			havingClauses = append(havingClauses, "("+strings.Join(groupHaving, " AND ")+")")
			havingParams = append(havingParams, groupHavingParams...)
		}
		params = append(params, groupParams...)
	}

	finalWhere := strings.Join(whereClauses, " AND ")
	finalHaving := ""
	if len(havingClauses) > 0 {
		finalHaving = "HAVING " + strings.Join(havingClauses, " AND ")
	}

	fmt.Println(req)
	fmt.Println("((((((((((((((((((((((req))))))))))))))))))))))")
	// check user property
	// userPropertySql := ""
	// if req.Property != "" {
	// 	userPropertySql = fmt.Sprintf(", COALESCE(JSON_UNQUOTE(JSON_EXTRACT(user_properties, '$.%s')), 'default') AS property", req.Property)
	// }

	// if req.Channel != "" {
	// 	if req.Channel == "email" {
	// 		finalWhere += " AND email is not NULL AND email != ''"
	// 	} else if req.Channel == "mobile" || req.Channel == "sms" {
	// 		finalWhere += " AND mobile is not NULL AND mobile != ''"
	// 	} else if req.Channel == "push" || req.Channel == "web_push" {
	// 		finalWhere += " AND id in (select external_user_id from notification_tokens where token is not null and token != '')"
	// 	}
	// }

	q := fmt.Sprintf(`
		SELECT nexora_id 
		FROM event_daily
		WHERE %s
		GROUP BY nexora_id
		%s
	`, finalWhere, finalHaving)
	fmt.Println(q)
	fmt.Println("Final Query ------------------------------------")
	if db.IsQueryLoggingEnabled() {
		log.Printf("[ClickHouse] Prefilter Query: %s | Params: %+v %+v\n", q, params, havingParams)
	}

	ctx := context.Background()
	rows, err := clickhouseConn.Query(ctx, q, append(params, havingParams...)...)
	if err != nil {
		fmt.Println(err)
		fmt.Println("((((((((err ins clickhouse))))))))")
		return nil, err
	}
	defer rows.Close()
	fmt.Println(rows)
	fmt.Println("((((((rows))))))")
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		fmt.Println(id)
		fmt.Println("((((id))))")
		out = append(out, id)
	}
	fmt.Println(out)
	return out, rows.Err()
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

// ---------------------- DEEP FILTER ----------------------
// func deepFilter(req models.SegmentPayload, nexoraIDs []string) ([]models.Member, error) {
// 	if len(req.Groups) == 0 || len(req.Groups[0].Filters) == 0 {
// 		return nil, fmt.Errorf("no filters provided")
// 	}

// 	f := req.Groups[0].Filters[0]
// 	log.Printf("%+v", f)
// 	log.Printf("%+v", nexoraIDs)
// 	nt, _, _, condStr, _ := normalizeFilter(f)
// 	log.Printf("++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++ condition string: %s", condStr)

// 	loc, _ := time.LoadLocation("Asia/Kolkata")
// 	start, end, err := utils.DeriveDateRange(nt.Operator, nt.Start, nt.End, nt.Value, nt.DayValue, nt.DayCount, time.Now(), loc)
// 	if err != nil {
// 		return nil, err
// 	}
// 	log.Printf("+++2222222+++++++++++++++++ ")
// 	eventCategory := mapEventType(f.EventType)
// 	eventName := f.EventName
// 	if f.ConditionBlock != nil {
// 		eventCategory = mapEventType(f.ConditionBlock.EventType)
// 		eventName = f.ConditionBlock.EventName
// 	}
// 	log.Printf("++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++ ")
// 	timeClause := "event_date >= toDate(?) AND event_date <= toDate(?)"
// 	timeParams := []any{start, end}
// 	if strings.ToLower(nt.Operator) == "before" {
// 		single := nt.Value
// 		if single == nil {
// 			single = nt.DayValue
// 		}
// 		timeClause = "event_date < toDate(?)"
// 		timeParams = []any{single}
// 	}

// 	inPh := makePlaceholders(len(nexoraIDs))
// 	baseWhere := fmt.Sprintf(`
// 		WHERE event_category = ?
// 		  AND event_name = ?
// 		  AND %s
// 		  AND nexora_id IN (%s)
// 	`, timeClause, inPh)

// 	var q string
// 	cond := f.Condition
// 	if f.ConditionBlock != nil {
// 		cond = f.ConditionBlock.Condition
// 	}

// 	if cond == "has_performed" {
// 		q = `SELECT DISTINCT nexora_id, client_id FROM events` + baseWhere
// 	} else if cond == "has_not_performed" {
// 		q = fmt.Sprintf(`
// 			SELECT DISTINCT nexora_id, client_id
// 			FROM customer_profiles
// 			WHERE nexora_id IN (%s)
// 			  AND nexora_id NOT IN (
// 				SELECT nexora_id FROM events %s
// 			  )
// 		`, inPh, baseWhere)
// 	} else {
// 		return nil, fmt.Errorf("unsupported condition: %s", cond)
// 	}

// 	params := []any{eventCategory, eventName}
// 	params = append(params, timeParams...)
// 	for _, id := range nexoraIDs {
// 		params = append(params, id)
// 	}

// 	conn := db.GetClickhouse()
// 	ctx := context.Background()
// 	rows, err := conn.Query(ctx, q, params...)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer rows.Close()

// 	var members []models.Member
// 	for rows.Next() {
// 		var m models.Member
// 		if err := rows.Scan(&m.NexoraID, &m.ClientID); err != nil {
// 			return nil, err
// 		}
// 		members = append(members, m)
// 	}
// 	return members, rows.Err()
// }

func deepFilter(req models.SegmentPayload, nexoraIDs []string) ([]models.Member, error) {
	if len(req.Groups) == 0 || len(req.Groups[0].Filters) == 0 {
		return nil, fmt.Errorf("no filters provided")
	}

	clientDBManager := db.NewClientDB()
	mysqlConn, _ := clientDBManager.GetMysqlDB(req.ClientID, req.ProjectID)
	clickhouseConn, _ := clientDBManager.GetCHDB(req.ClientID, req.ProjectID)

	// Find the first event filter in the first group (or any group if you prefer)
	var eventFilter *models.Filter
	for gi := range req.Groups {
		for fi := range req.Groups[gi].Filters {
			f := &req.Groups[gi].Filters[fi]
			// prefer explicit event category
			if strings.ToLower(strings.TrimSpace(f.FilterCategory)) == "event" {
				eventFilter = f
				break
			}
			// fallback: if ConditionBlock has time or event_name/event_type info treat as event
			if f.ConditionBlock != nil && (f.ConditionBlock.Time.Operator != "" ||
				f.ConditionBlock.EventName != "" || f.ConditionBlock.EventType != "") {
				eventFilter = f
				break
			}
		}
		if eventFilter != nil {
			break
		}
	}

	if eventFilter == nil {
		// No event filter found — nothing to deep-filter. Return empty slice or prefiltered members.
		// Prefer returning prefiltered members (nexoraIDs -> members) so caller gets final list.
		var members []models.Member
		for _, id := range nexoraIDs {
			members = append(members, models.Member{NexoraID: id})
		}
		return members, nil
	}

	// use the found eventFilter from here on
	f := *eventFilter
	log.Printf("%+v", f)
	log.Printf("%+v", nexoraIDs)

	nt, _, _, condStr, _ := normalizeFilter(f)
	log.Printf("++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++ condition string: %s", condStr)

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

	// build time clause — handle "on" / "before" / default (range)
	var timeClause string
	var timeParams []any

	op := strings.ToLower(strings.TrimSpace(nt.Operator))
	switch op {
	case "before":
		single := nt.Value
		if single == nil {
			single = nt.DayValue
		}
		timeClause = "event_date < toDate(?)"
		timeParams = []any{single}
	case "on":
		single := nt.Value
		if single == nil {
			single = nt.DayValue
		}
		// equality by date; if event_date is DateTime you can expand to full day
		timeClause = "event_date = toDate(?)"
		timeParams = []any{single}
	default:
		timeClause = "event_date >= toDate(?) AND event_date <= toDate(?)"
		timeParams = []any{start, end}
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
	fmt.Println(req)
	fmt.Println("((((((((((((((((((((((req))))))))))))))))))))))")
	// check user property

	if cond == "has_performed" {
		q = `SELECT DISTINCT nexora_id, client_id FROM events` + baseWhere
	} else if cond == "has_not_performed" {
		q = fmt.Sprintf(`
			SELECT DISTINCT nexora_id, client_id FROM events
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

	ctx := context.Background()
	fmt.Println(q)
	fmt.Println(params...)
	fmt.Println("((((((((((((((((((((((params...))))))))))))))))))))))")
	rows, err := clickhouseConn.Query(ctx, q, params...)
	if err != nil {
		return nil, err
	}
	fmt.Println(rows)
	fmt.Println("(((((((((((((((((((((((((((((rows **********)))))))))))))))))))))))))))))")
	defer rows.Close()

	var members []models.Member
	for rows.Next() {
		var m models.Member
		if err := rows.Scan(&m.NexoraID, &m.ClientID); err != nil {
			return nil, err
		}
		// get the customer properties and check reachanilities here
		userPropertySql := ""
		where := ""

		if req.Property != "" {
			userPropertySql = fmt.Sprintf(
				"COALESCE(JSON_UNQUOTE(JSON_EXTRACT(user_properties, '$.%s')), 'default') AS property",
				req.Property,
			)
		} else {
			userPropertySql = "'' AS property"
		}

		if req.Channel != "" {
			if req.Channel == "email" {
				where += " AND email IS NOT NULL AND email != ''"
			} else if req.Channel == "mobile" || req.Channel == "sms" {
				where += " AND mobile IS NOT NULL AND mobile != ''"
			} else if req.Channel == "push" || req.Channel == "web_push" {
				where += " AND id IN (SELECT external_user_id FROM notification_tokens WHERE token IS NOT NULL AND token != '')"
			}
		}

		q := fmt.Sprintf(`
			SELECT %s
			FROM customer_profiles
			WHERE id IN (
				SELECT customer_profile_id
				FROM nexora_profiles
				WHERE nexora_id = '%s'
			) %s
			LIMIT 1
		`, userPropertySql, m.NexoraID, where)

		rows, err := mysqlConn.Query(q)
		if err != nil {
			return nil, err
		}

		propertyValue := ""
		count := 0
		if rows.Next() {
			count++
			if err := rows.Scan(&propertyValue); err != nil {
				rows.Close()
				return nil, err
			}
		}

		if count > 0 {
			m.Property = propertyValue
		}
		fmt.Println(m)
		fmt.Println(m.NexoraID)
		fmt.Println(m.ClientID)
		fmt.Println("((((((((((((((((((((((((((m))))))))))))))))))))))))))")
		rows.Close() // Don't defer inside loop
		members = append(members, m)
	}
	return members, rows.Err()
}

// ---------------------- HELPERS ----------------------
func mapEventType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "system events", "system_event", "system":
		return "system_event"
	case "custom events", "custom_event", "custom":
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
