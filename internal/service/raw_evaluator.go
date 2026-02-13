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

func isNegativeSemantic(op string, val interface{}) bool {
	op = strings.ToLower(strings.TrimSpace(op))

	switch op {
	case "not in", "not like", "!=", "is null", "is not null":
		return true
	}

	// empty string checks
	if s, ok := val.(string); ok {
		if (op == "=" || op == "!=") && s == "" {
			return true
		}
	}

	return false
}

func handleTypedRule(
	r models.Rule,
	where *[]string,
	having *[]string,
	scope *string,
	includeAnonymouseUsers string,
) {
	op := getCHEquivalentOperator(r.Operator)

	// Nested rule (user / context)
	if nested, ok := r.Value.(models.QueryBlock); ok {
		*scope = r.Field
		for _, nr := range nested.Rules {
			handleTypedRule(nr, where, having, scope, includeAnonymouseUsers)
		}
		return
	}

	if isNegativeSemantic(op, r.Value) {
		fmt.Println("Inside negative semantic ............")
		includeAnonymouseUsers = "yes"
	}

	if *scope == "user" {

		cp := getCustomerProfileEquivalentField(r.Field)
		opLower := strings.ToLower(op)

		if opLower == "like" || opLower == "not like" {

			if strings.ToLower(r.Operator) == "beginswith" || strings.ToLower(r.Operator) == "doesnotendwith" {
				*having = append(*having,
					fmt.Sprintf("%s %s '%v%%'", cp, op, r.Value),
				)
			} else if strings.ToLower(r.Operator) == "endswith" || strings.ToLower(r.Operator) == "doesnotbeginwith" {
				*having = append(*having,
					fmt.Sprintf("%s %s '%%%v'", cp, op, r.Value),
				)
			} else {
				*having = append(*having,
					fmt.Sprintf("%s %s '%%%v%%'", cp, op, r.Value),
				)
			}

		} else if opLower == "is null" || opLower == "is not null" {

			*having = append(*having,
				fmt.Sprintf("%s %s", cp, op),
			)

		} else if opLower == "in" || opLower == "not in" {

			var valuesArr []string

			switch v := r.Value.(type) {
			case []string:
				for _, val := range v {
					valuesArr = append(valuesArr, fmt.Sprintf("'%v'", val))
				}
			case string:
				valuesArr = []string{fmt.Sprintf("'%v'", v)}
			default:
				valuesArr = []string{fmt.Sprintf("'%v'", v)}
			}

			values := strings.Join(valuesArr, ", ")

			*having = append(*having,
				fmt.Sprintf("%s %s (%v)", cp, op, values),
			)

		} else {

			*having = append(*having,
				fmt.Sprintf("%s %s '%v'", cp, op, r.Value),
			)
		}

	} else {

		ev := getEventsEquivalentField(r.Field)
		opLower := strings.ToLower(op)

		if opLower == "like" || opLower == "not like" {

			if strings.ToLower(r.Operator) == "beginswith" || strings.ToLower(r.Operator) == "doesnotendwith" {
				*where = append(*where,
					fmt.Sprintf("%s %s '%v%%'", ev, op, r.Value),
				)
			} else if strings.ToLower(r.Operator) == "endswith" || strings.ToLower(r.Operator) == "doesnotbeginwith" {
				*where = append(*where,
					fmt.Sprintf("%s %s '%%%v'", ev, op, r.Value),
				)
			} else {
				*where = append(*where,
					fmt.Sprintf("%s %s '%%%v%%'", ev, op, r.Value),
				)
			}

		} else if opLower == "is null" || opLower == "is not null" {

			*where = append(*where,
				fmt.Sprintf("%s %s", ev, op),
			)

		} else if opLower == "in" || opLower == "not in" {

			var valuesArr []string

			switch v := r.Value.(type) {
			case []string:
				for _, val := range v {
					valuesArr = append(valuesArr, fmt.Sprintf("'%v'", val))
				}
			case string:
				valuesArr = []string{fmt.Sprintf("'%v'", v)}
			default:
				valuesArr = []string{fmt.Sprintf("'%v'", v)}
			}

			values := strings.Join(valuesArr, ", ")

			*where = append(*where,
				fmt.Sprintf("%s %s (%v)", ev, op, values),
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
	case "in":
		return "in"
	case "notin":
		return "not in"
	case "null":
		return "is null"
	case "notnull":
		return "is not null"
	case "beginswith", "doesnotendwith", "endswith", "doesnotbeginwith":
		return "like"
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
	// boolean for checking whether the conditions has to fetch all non-matching users in scenarios like not like, not in
	includeAnonymouseUsers := "no"
	// append project_id_state in groupWhere
	groupHaving = append(groupHaving, fmt.Sprintf("argMaxMerge(cp.project_id_state) = '%s'", req.ProjectID))

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
						handleTypedRule(r, &where, &having, &field, includeAnonymouseUsers)
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
						handleTypedRule(r, &where, &having, &field, includeAnonymouseUsers)
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
	if req.LimitStatement != "" {
		limitAndOffsets = req.LimitStatement
	}
	withStatement := ""
	countStatement := ""
	whereNonAggregateStatement := ""

	// check for having
	if len(groupHaving) > 0 {
		finalHaving = "HAVING " + strings.Join(groupHaving, " "+groupCondition+" ")
	} else {
		finalHaving = ""
	}

	outerSelectStatement := "SELECT argMax(id, id) AS customer_profile_id, argMax(external_user_id, id) as external_user_id, nexora_id, argMax(email, id) AS email, argMax(mobile, id) AS mobile, argMax(name, id) AS name, argMax(project_id, id) AS project_id, argMax(client_id, id) AS client_id, argMax(user_properties, id) AS user_properties, argMax(gender, id) AS gender FROM "

	selectStatement := "SELECT cp.id, cp.external_user_id, argMaxMerge(cp.nexora_id_state) AS nexora_id, argMaxMerge(cp.email_state) AS email, argMaxMerge(cp.mobile_state) AS mobile, argMaxMerge(cp.name_state) AS name, argMaxMerge(cp.project_id_state) AS project_id, argMaxMerge(cp.client_id_state) AS client_id, argMaxMerge(cp.user_properties_state) AS user_properties, JSONExtractString(argMaxMerge(cp.user_properties_state), 'gender') AS gender"
	if req.Source == "campaign_service" {
		outerSelectStatement = "SELECT argMax(id, id) AS customer_profile_id, argMax(external_user_id, id) as external_user_id, nexora_id, argMax(email, id) AS email, argMax(mobile, id) AS mobile, argMax(name, id) AS name, argMax(project_id, id) AS project_id, argMax(client_id, id) AS client_id, argMax(user_properties, id) AS user_properties, argMax(property, id) AS property FROM "
		selectStatement = fmt.Sprintf("SELECT cp.id, cp.external_user_id, argMaxMerge(cp.nexora_id_state) AS nexora_id, argMaxMerge(cp.email_state) AS email, argMaxMerge(cp.mobile_state) AS mobile, argMaxMerge(cp.name_state) AS name, argMaxMerge(cp.project_id_state) AS project_id, argMaxMerge(cp.client_id_state) AS client_id, argMaxMerge(cp.user_properties_state) AS user_properties, coalesce( nullIf(JSONExtractString(argMaxMerge(cp.user_properties_state), '%s'), ''), 'default') AS property", req.Property)
	}

	if len(req.NexoraIDs) > 0 {
		nexoraIn := buildInCondition("argMaxMerge(cp.nexora_id_state)", req.NexoraIDs)
		if finalHaving == "" {
			strings.Replace(nexoraIn, "AND", "HAVING", 1)
			finalHaving = nexoraIn
		} else {
			finalHaving += nexoraIn
		}
	}

	if len(groupWhere) > 0 {
		// select statemets if it has event filters
		withSelectStatement := "SELECT cp.id, cp.external_user_id, argMaxMerge(cp.nexora_id_state) AS nexora_id, argMaxMerge(cp.email_state) AS email, argMaxMerge(cp.mobile_state) AS mobile, argMaxMerge(cp.name_state) AS name, argMaxMerge(cp.project_id_state) AS project_id, argMaxMerge(cp.client_id_state) AS client_id, argMaxMerge(cp.user_properties_state) AS user_properties, JSONExtractString(argMaxMerge(cp.user_properties_state), 'gender') AS gender"
		finalWhere = "WHERE " + strings.Join(groupWhere, " "+groupCondition+" ")
		withStatement = fmt.Sprintf("WITH event_users AS (SELECT DISTINCT ev.nexora_id FROM events ev INNER JOIN event_daily ed ON ev.event_name = ed.event_name AND ev.nexora_id = ed.nexora_id %s), cp_base AS (%s from customer_profiles_latest cp %s group by cp.id, cp.external_user_id %s)", finalWhere, withSelectStatement, whereNonAggregateStatement, finalHaving)
		selectStatement := "SELECT nexora_id, argMax(id, id) AS customer_profile_id, argMax(external_user_id, id) AS external_user_id, argMax(email, id) AS email, argMax(mobile, id) AS mobile, argMax(name, id) AS name, argMax(project_id, id) AS project_id, argMax(client_id, id) AS client_id, argMax(user_properties, id) AS user_properties, argMax(gender, id) AS gender FROM cp_base GROUP BY nexora_id"
		joinStatement = "event_users eu INNER JOIN cp_resolved cp ON eu.nexora_id = cp.nexora_id"
		overallSelectStatement = fmt.Sprintf("%s, cp_resolved AS (%s) select cp.nexora_id as nexora_id, cp.customer_profile_id, cp.external_user_id, cp.email, cp.mobile, cp.name, cp.project_id, cp.client_id, cp.user_properties, cp.property from %s order by customer_profile_id %s", withStatement, selectStatement, joinStatement, limitAndOffsets)
		countStatement = fmt.Sprintf("%s, cp_resolved AS (%s) select COUNT(*) AS total_count FROM %s %s", withStatement, selectStatement, joinStatement, limitAndOffsets)

	} else {
		joinStatement = "customer_profiles_latest cp"
		overallSelectStatement = fmt.Sprintf("%s ( %s from %s %s group by cp.id, cp.external_user_id %s ) GROUP BY nexora_id order by customer_profile_id %s", outerSelectStatement, selectStatement, joinStatement, whereNonAggregateStatement, finalHaving, limitAndOffsets)
		countStatement = fmt.Sprintf("SELECT COUNT(*) AS total_count FROM ( %s ( %s from %s %s group by cp.id, cp.external_user_id %s ) GROUP BY nexora_id %s) as sub", outerSelectStatement, selectStatement, joinStatement, whereNonAggregateStatement, finalHaving, limitAndOffsets)
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
	fmt.Println(overallSelectStatement)
	fmt.Println("(((((((overallSelectStatement)))))))")
	fmt.Println(countStatement)
	fmt.Println("((((countStatement))))")
	query := map[string]string{
		"where_statement":               finalWhere,
		"join_statement":                joinStatement,
		"having_statement":              finalHaving,
		"select_statement":              selectStatement,
		"with_statement":                withStatement,
		"count_statement":               countStatement,
		"where_non_aggregate_statement": whereNonAggregateStatement,
		"overall_statement":             overallSelectStatement,
		"include_anonymous_users":       includeAnonymouseUsers,
	}

	return map[string]interface{}{
		"query": query,
		"count": count,
	}, nil
}
