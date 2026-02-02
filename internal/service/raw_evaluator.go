package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nexora/nexora_segmentation/internal/db"
	"github.com/nexora/nexora_segmentation/internal/models"
)

func daysFromNow(dateStr string) (int, error) {
	// Parse the date string in YYYY-MM-DD format
	targetDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return 0, fmt.Errorf("invalid date format: %v", err)
	}

	// Get current date in UTC (or local, depending on your logic)
	now := time.Now().UTC()

	// Calculate difference
	duration := targetDate.Sub(now)

	// Convert duration to days (rounding down)
	days := int(duration.Hours() / 24)

	return days, nil
}

func getTimeConditionsTyped(tc *models.TimeCondition) string {
	if tc == nil {
		return ""
	}

	op := strings.ToLower(tc.Operator)
	now := time.Now().UTC()

	var days int
	var err error
	switch v := tc.Value.(type) {
	case string:
		days, err = daysFromNow(v)
	case float64:
		// do nothing
	}
	if err != nil && op != "between" {
		fmt.Println(fmt.Errorf("invalid date format: %v", err))
		return ""
	}

	switch op {

	case "last_n_days":
		return fmt.Sprintf(
			"ed.event_date <= %s",
			chDateTime(now.AddDate(0, 0, -days)),
		)

	case "next_n_days":
		return fmt.Sprintf(
			"ed.event_date >= %s",
			chDateTime(now.AddDate(0, 0, days)),
		)

	case "on":
		return fmt.Sprintf(
			"ed.event_date = %s",
			chDateTime(now.AddDate(0, 0, days)),
		)

	case "before":
		return fmt.Sprintf(
			"ed.event_date < %s",
			chDateTime(now.AddDate(0, 0, days)),
		)

	case "after":
		return fmt.Sprintf(
			"ed.event_date > %s",
			chDateTime(now.AddDate(0, 0, days)),
		)

	case "between":
		start, err1 := time.Parse("2006-01-02", tc.StartDate)
		end, err2 := time.Parse("2006-01-02", tc.EndDate)
		if err1 != nil || err2 != nil {
			return ""
		}
		return fmt.Sprintf(
			"ed.event_date BETWEEN %s AND %s",
			chDateTime(start), chDateTime(end),
		)

	default:
		return ""
	}
}

func getCountConditionsTyped(cc *models.CountCondition) string {
	if cc == nil {
		return ""
	}

	op := strings.ToLower(cc.Operator)

	var count int
	switch v := cc.Value.(type) {
	case int:
		count = v
	case string:
		d, _ := strconv.Atoi(v)
		count = d
	}
	switch op {

	case "greater_than":
		return fmt.Sprintf("ed.event_count > %d", count)

	case "greater_than_or_equal":
		return fmt.Sprintf("ed.event_count >= %d", count)

	case "less_than":
		return fmt.Sprintf("ed.event_count < %d", count)

	case "less_than_or_equal":
		return fmt.Sprintf("ed.event_count <= %d", count)

	case "between":
		return fmt.Sprintf(
			"ed.event_count BETWEEN %d AND %d",
			cc.Min, cc.Max,
		)

	case "equal":
		return fmt.Sprintf("ed.event_count = %d", count)

	default:
		return ""
	}
}

func handleTypedRule(
	r models.Rule,
	where *[]string,
	having *[]string,
	scope *string,
) {
	op := getCHEquivalentOperator(r.Operator)

	// Nested rule (user / context)
	if nested, ok := r.Value.(models.QueryBlock); ok {
		*scope = r.Field
		for _, nr := range nested.Rules {
			handleTypedRule(nr, where, having, scope)
		}
		return
	}

	if *scope == "user" {
		cp := getCustomerProfileEquivalentField(r.Field)
		if op == "like" || op == "not like" {
			*having = append(*having,
				fmt.Sprintf("%s %s '%%%v%%'", cp, op, r.Value),
			)
		} else {
			*having = append(*having,
				fmt.Sprintf("%s %s '%v'", cp, op, r.Value),
			)
		}
	} else {
		ev := getEventsEquivalentField(r.Field)
		if op == "like" || op == "not like" {
			*where = append(*where,
				fmt.Sprintf("%s %s '%%%v%%'", ev, op, r.Value),
			)
		} else {
			*where = append(*where,
				fmt.Sprintf("%s %s '%v'", ev, op, r.Value),
			)
		}
	}
}

func getCHEquivalentOperator(op string) string {
	switch strings.ToLower(op) {
	case "=", "eq":
		return "="
	case "!=", "neq":
		return "!="
	case ">", "gt":
		return ">"
	case "<", "lt":
		return "<"
	case ">=", "gte":
		return ">="
	case "<=", "lte":
		return "<="
	case "like", "contains":
		return "like"
	case "doesNotContain", "doesnotcontain":
		return "not like"
	default:
		return ""
	}
}

func getCustomerProfileEquivalentField(field string) string {
	// Fields that are NOT aggregate states (safe to GROUP BY)
	groupableColumns := map[string]bool{
		"id":               true,
		"external_user_id": true,
	}

	if groupableColumns[field] {
		return fmt.Sprintf("cp.%s", field)
	}

	// Known argMax state columns
	switch field {
	case "email":
		return "argMaxMerge(cp.email_state)"
	case "mobile":
		return "argMaxMerge(cp.mobile_state)"
	case "name":
		return "argMaxMerge(cp.name_state)"
	case "client_id":
		return "argMaxMerge(cp.client_id_state)"
	case "project_id":
		return "argMaxMerge(cp.project_id_state)"
	case "updated_at":
		return "argMaxMerge(cp.updated_at_state)"
	}

	// Dynamic JSON fields from user_properties_state
	return fmt.Sprintf(
		"JSONExtractString(argMaxMerge(cp.user_properties_state), '%s')",
		field,
	)
}

func getEventsEquivalentField(field string) string {
	directFields := map[string]bool{
		"event_name": true, "timestamp": true,
		"sdk_version": true, "device_platform": true,
		"device_app_platform": true, "device_type": true,
		"device_os_name": true, "device_os_version": true,
		"device_browser": true, "device_browser_version": true,
		"device_user_agent": true,
		"app_name":          true, "app_version": true,
		"app_build_number": true,
		"context_locale":   true, "context_timezone": true,
	}

	if directFields[field] {
		return fmt.Sprintf("ev.%s", field)
	}

	jsonPathMap := map[string]string{
		"sdk_version":      "app.sdk_version",
		"app_version":      "app.version",
		"app_build_number": "app.build_number",
		"context_locale":   "context.locale",
		"context_timezone": "context.timezone",
	}

	if path, ok := jsonPathMap[field]; ok {
		return fmt.Sprintf(
			"JSONExtractString(arrayElement(JSONExtract(ev.raw_payload, 'Array(JSON)'), 1), '%s')",
			path,
		)
	}

	return fmt.Sprintf("JSONExtractString(ev.event_properties, '%s')", field)
}

func chDateTime(t time.Time) string {
	return fmt.Sprintf(
		"toDateTime('%s', 'UTC')",
		t.UTC().Format("2006-01-02"),
	)
}

func buildInCondition(column string, values []string) string {
	if len(values) == 0 {
		return ""
	}

	escaped := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			escaped = append(escaped, fmt.Sprintf("'%s'", v))
		}
	}

	if len(escaped) == 0 {
		return ""
	}

	return fmt.Sprintf(" AND %s IN (%s) ", column, strings.Join(escaped, ","))
}

func EvaluteRaw(req models.SegmentNewPayload) (map[string]interface{}, error) {
	groupCondition := strings.ToUpper(req.GroupCondition)
	if groupCondition == "" {
		groupCondition = "AND"
	}

	groupWhere := []string{}
	groupHaving := []string{}

	for _, group := range req.Groups {
		whereClauses := []string{}
		havingClauses := []string{}

		for _, filter := range group.Filters {

			switch filter.FilterCategory {

			// ---------- EVENT ----------
			case "event":
				var ec models.EventCondition
				if err := json.Unmarshal(filter.Condition, &ec); err != nil {
					fmt.Println(err)
					fmt.Println("(((err)))")
					continue
				}

				// query rules
				if ec.Query != nil {
					where := []string{}
					having := []string{}
					field := ""
					for _, r := range ec.Query.Rules {
						handleTypedRule(r, &where, &having, &field)
					}
					if len(where) > 0 {
						whereClauses = append(
							whereClauses,
							fmt.Sprintf("( %s )",
								strings.Join(where, " "+strings.ToUpper(ec.Query.Combinator)+" "),
							),
						)
					}
					if len(having) > 0 {
						fmt.Println(having)
						fmt.Println("((((having))))")
						havingClauses = append(
							havingClauses,
							fmt.Sprintf("( %s )",
								strings.Join(having, " "+strings.ToUpper(ec.Query.Combinator)+" "),
							),
						)
					}
				}

				// time
				if ec.Time != nil {
					fmt.Println("Time condition")
					timeCondition := getTimeConditionsTyped(ec.Time)
					fmt.Println(timeCondition)
					fmt.Println("((((timeCondition))))")
					if timeCondition != "" {
						whereClauses = append(whereClauses, timeCondition)
					}
				}

				// count
				if ec.Count != nil {
					fmt.Println("Count condition")
					countCondition := getCountConditionsTyped(ec.Count)
					if countCondition != "" {
						whereClauses = append(whereClauses, getCountConditionsTyped(ec.Count))
					}
				}

				// event name
				whereClauses = append(
					whereClauses,
					fmt.Sprintf("ed.event_name = '%s'", ec.EventName),
				)

				// ---------- USER PROPERTY ----------
			case "user_property":
				var up models.UserPropertyCondition
				if err := json.Unmarshal(filter.Condition, &up); err != nil {
					continue
				}

				if up.UserPropertyQuery != nil {
					where := []string{}
					having := []string{}
					field := "user"
					for _, r := range up.UserPropertyQuery.Rules {
						handleTypedRule(r, &where, &having, &field)
					}
					if len(where) > 0 {
						whereClauses = append(
							whereClauses,
							fmt.Sprintf("( %s )",
								strings.Join(where, " "+strings.ToUpper(up.UserPropertyQuery.Combinator)+" "),
							),
						)
					}
					if len(having) > 0 {
						fmt.Println(having)
						fmt.Println("((((having))))")
						havingClauses = append(
							havingClauses,
							fmt.Sprintf("( %s )",
								strings.Join(having, " "+strings.ToUpper(up.UserPropertyQuery.Combinator)+" "),
							),
						)
					}
				}
			}
		}

		if len(whereClauses) > 0 {
			groupWhere = append(
				groupWhere,
				strings.Join(whereClauses, " "+strings.ToUpper(group.MatchMode)+" "),
			)
		}
		if len(havingClauses) > 0 {
			groupHaving = append(
				groupHaving,
				strings.Join(havingClauses, " "+strings.ToUpper(group.MatchMode)+" "),
			)
		}
	}

	finalWhere := ""
	finalHaving := ""
	joinStatement := ""
	overallSelectStatement := ""
	limitAndOffsets := ""
	withStatement := ""
	countStatement := ""
	whereNonAggregateStatement := ""

	// check for having
	if len(groupHaving) > 0 {
		finalHaving = "HAVING " + strings.Join(groupHaving, " "+groupCondition+" ")
	} else {
		finalHaving = ""
	}

	// check nexora_ids in condition
	if len(req.NexoraIDs) > 0 {
		whereNonAggregateStatement = buildInCondition("np.nexora_id", req.NexoraIDs)
	}

	selectStatement := "SELECT cp.id AS customer_profile_id, any(np.nexora_id) AS nexora_id, JSONExtractString(argMaxMerge(cp.user_properties_state), 'gender') AS gender"
	if req.Source == "campaign_service" {
		selectStatement = fmt.Sprintf("SELECT cp.id AS customer_profile_id, any(np.nexora_id) AS nexora_id, JSONExtractString(argMaxMerge(cp.user_properties_state), '%s') AS property", req.Property)
	}
	// check fot where
	if len(groupWhere) > 0 {
		finalWhere = "WHERE " + strings.Join(groupWhere, " "+groupCondition+" ")
		withStatement = fmt.Sprintf("WITH event_users AS (SELECT DISTINCT ev.nexora_id FROM events ev INNER JOIN event_daily ed ON ev.event_name = ed.event_name AND ev.nexora_id = ed.nexora_id %s)", finalWhere)
		joinStatement = "event_users eu ANY INNER JOIN nexora_profiles_latest np ON eu.nexora_id = np.nexora_id ANY INNER JOIN customer_profiles_latest cp ON np.customer_profile_id = cp.id"
		overallSelectStatement = fmt.Sprintf("%s %s from %s %s group by cp.id %s order by customer_profile_id %s", withStatement, selectStatement, joinStatement, whereNonAggregateStatement, finalHaving, limitAndOffsets)
		countStatement = fmt.Sprintf("%s SELECT COUNT(*) AS total_count FROM (%s from %s %s group by cp.id %s order by customer_profile_id %s) as sub", withStatement, selectStatement, joinStatement, whereNonAggregateStatement, finalHaving, limitAndOffsets)
	} else {
		joinStatement = "customer_profiles_latest cp LEFT JOIN nexora_profiles_latest np ON cp.id = np.customer_profile_id"
		overallSelectStatement = fmt.Sprintf("%s from %s %s group by cp.id %s order by customer_profile_id %s as sub", selectStatement, joinStatement, whereNonAggregateStatement, finalHaving, limitAndOffsets)
		countStatement = fmt.Sprintf("SELECT COUNT(*) AS total_count FROM ( %s from %s %s group by cp.id %s order by customer_profile_id %s) as sub", selectStatement, joinStatement, whereNonAggregateStatement, finalHaving, limitAndOffsets)
	}

	var count uint64
	if req.IsNeedCount {
		clientDBManager := db.NewClientDB()
		clickhouseConn, err := clientDBManager.GetCHDB(req.ClientID, req.ProjectID)
		fmt.Println(countStatement)
		fmt.Println("((((countStatement))))")
		row := clickhouseConn.QueryRow(context.Background(), countStatement)

		// Scan the value into the variable
		err = row.Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("failed to get count: %v", err)
		}

	}

	query := map[string]string{
		"where_statement":               finalWhere,
		"join_statement":                joinStatement,
		"having_statement":              finalHaving,
		"select_statement":              selectStatement,
		"with_statement":                withStatement,
		"count_statement":               countStatement,
		"where_non_aggregate_statement": whereNonAggregateStatement,
		"overall_statement":             overallSelectStatement,
	}

	return map[string]interface{}{
		"query": query,
		"count": count,
	}, nil
}
