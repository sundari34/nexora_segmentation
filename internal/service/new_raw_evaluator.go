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

// ─── helpers ────────────────────────────────────────────────────────────────

func daysFromNow(dateStr string) (int, error) {
	targetDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return 0, fmt.Errorf("invalid date format: %v", err)
	}
	now := time.Now().UTC()
	duration := targetDate.Sub(now)
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
		days = int(v)
	}
	if err != nil && op != "between" {
		return ""
	}

	switch op {
	case "last_n_days":
		return fmt.Sprintf("ed.event_date <= %s", chDateTime(now.AddDate(0, 0, -days)))
	case "next_n_days":
		return fmt.Sprintf("ed.event_date >= %s", chDateTime(now.AddDate(0, 0, days)))
	case "on":
		return fmt.Sprintf("ed.event_date = %s", chDateTime(now.AddDate(0, 0, days)))
	case "before":
		return fmt.Sprintf("ed.event_date < %s", chDateTime(now.AddDate(0, 0, days)))
	case "after":
		return fmt.Sprintf("ed.event_date > %s", chDateTime(now.AddDate(0, 0, days)))
	case "between":
		start, err1 := time.Parse("2006-01-02", tc.StartDate)
		end, err2 := time.Parse("2006-01-02", tc.EndDate)
		if err1 != nil || err2 != nil {
			return ""
		}
		return fmt.Sprintf("ed.event_date BETWEEN %s AND %s", chDateTime(start), chDateTime(end))
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
	case float64:
		count = int(v)
	case string:
		count, _ = strconv.Atoi(v)
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
		return fmt.Sprintf("ed.event_count BETWEEN %d AND %d", cc.Min, cc.Max)
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
	if s, ok := val.(string); ok {
		if (op == "=" || op == "!=") && s == "" {
			return true
		}
	}
	return false
}

func handleTypedRule(r models.Rule, where *[]string, having *[]string, scope *string, includeAnonymouseUsers string) {
	op := getCHEquivalentOperator(r.Operator)

	if nested, ok := r.Value.(models.QueryBlock); ok {
		*scope = r.Field
		for _, nr := range nested.Rules {
			handleTypedRule(nr, where, having, scope, includeAnonymouseUsers)
		}
		return
	}

	if isNegativeSemantic(op, r.Value) {
		includeAnonymouseUsers = "yes"
	}

	if *scope == "user" {
		cp := getCustomerProfileEquivalentField(r.Field)
		opLower := strings.ToLower(op)

		if opLower == "like" || opLower == "not like" {
			switch strings.ToLower(r.Operator) {
			case "beginswith", "doesnotendwith":
				*having = append(*having, fmt.Sprintf("%s %s '%v%%'", cp, op, r.Value))
			case "endswith", "doesnotbeginwith":
				*having = append(*having, fmt.Sprintf("%s %s '%%%v'", cp, op, r.Value))
			default:
				*having = append(*having, fmt.Sprintf("%s %s '%%%v%%'", cp, op, r.Value))
			}
		} else if opLower == "is null" || opLower == "is not null" {
			var condition string
			if opLower == "is not null" {
				condition = fmt.Sprintf("(%s IS NOT NULL AND %s != '' AND %s != '0' AND LOWER(%s) != 'none')", cp, cp, cp, cp)
			} else {
				condition = fmt.Sprintf("(%s IS NULL OR %s = '' OR %s = '0' OR LOWER(%s) = 'none')", cp, cp, cp, cp)
			}
			*having = append(*having, condition)
		} else if opLower == "in" || opLower == "not in" {
			var valuesArr []string
			switch v := r.Value.(type) {
			case []string:
				for _, val := range v {
					valuesArr = append(valuesArr, fmt.Sprintf("'%v'", val))
				}
			default:
				valuesArr = []string{fmt.Sprintf("'%v'", v)}
			}
			*having = append(*having, fmt.Sprintf("%s %s (%v)", cp, op, strings.Join(valuesArr, ", ")))
		} else {
			*having = append(*having, fmt.Sprintf("%s %s '%v'", cp, op, r.Value))
		}
	} else {
		ev := getEventsEquivalentField(r.Field)
		opLower := strings.ToLower(op)

		if opLower == "like" || opLower == "not like" {
			switch strings.ToLower(r.Operator) {
			case "beginswith", "doesnotendwith":
				*where = append(*where, fmt.Sprintf("%s %s '%v%%'", ev, op, r.Value))
			case "endswith", "doesnotbeginwith":
				*where = append(*where, fmt.Sprintf("%s %s '%%%v'", ev, op, r.Value))
			default:
				*where = append(*where, fmt.Sprintf("%s %s '%%%v%%'", ev, op, r.Value))
			}
		} else if opLower == "is null" || opLower == "is not null" {
			var condition string
			if opLower == "is not null" {
				condition = fmt.Sprintf("(%s IS NOT NULL AND %s != '' AND %s != '0' AND LOWER(%s) != 'none')", ev, ev, ev, ev)
			} else {
				condition = fmt.Sprintf("(%s IS NULL OR %s = '' OR %s = '0' OR LOWER(%s) = 'none')", ev, ev, ev, ev)
			}
			*where = append(*where, condition)
		} else if opLower == "in" || opLower == "not in" {
			var valuesArr []string
			switch v := r.Value.(type) {
			case []string:
				for _, val := range v {
					valuesArr = append(valuesArr, fmt.Sprintf("'%v'", val))
				}
			default:
				valuesArr = []string{fmt.Sprintf("'%v'", v)}
			}
			*where = append(*where, fmt.Sprintf("%s %s (%v)", ev, op, strings.Join(valuesArr, ", ")))
		} else {
			*where = append(*where, fmt.Sprintf("%s %s '%v'", ev, op, r.Value))
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
	case "doesnotcontain":
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
	switch field {
	case "id":
		return "customer_profile_id"
	case "external_user_id":
		return "external_user_id"
	case "email":
		return "email"
	case "mobile":
		return "mobile"
	case "name":
		return "name"
	case "client_id":
		return "client_id"
	case "project_id":
		return "project_id"
	case "updated_at":
		return "updated_at"
	}
	return fmt.Sprintf("JSONExtractString(user_properties, '%s')", field)
}

func getEventsEquivalentField(field string) string {
	directFields := map[string]bool{
		"event_name": true, "timestamp": true, "sdk_version": true,
		"device_platform": true, "device_app_platform": true, "device_type": true,
		"device_os_name": true, "device_os_version": true, "device_browser": true,
		"device_browser_version": true, "device_user_agent": true,
		"app_name": true, "app_version": true, "app_build_number": true,
		"context_locale": true, "context_timezone": true,
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
		return fmt.Sprintf("JSONExtractString(arrayElement(JSONExtract(ev.raw_payload, 'Array(JSON)'), 1), '%s')", path)
	}
	return fmt.Sprintf("JSONExtractString(ev.event_properties, '%s')", field)
}

func chDateTime(t time.Time) string {
	return fmt.Sprintf("toDateTime('%s', 'UTC')", t.UTC().Format("2006-01-02"))
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

// ─── cp resolution CTE ───────────────────────────────────────────────────────
// New schema: ORDER BY nexora_id, external_user_id is a state
// Two levels:
//   inner  → resolve *Merge states per nexora_id → flat row with identity_key
//   outer  → GROUP BY identity_key to collapse same user across devices

func cpResolutionCTEs(projectID, property string) string {
	return fmt.Sprintf(`cp_inner AS (
    SELECT
        nexora_id,
        multiIf(
            argMaxMerge(external_user_id_state) != 'none',
            argMaxMerge(external_user_id_state),
            nexora_id
        )                                                           AS identity_key,
        maxMerge(id_state)                                          AS id,
        argMaxMerge(external_user_id_state)                         AS external_user_id,
        argMaxMerge(email_state)                                    AS email,
        argMaxMerge(mobile_state)                                   AS mobile,
        argMaxMerge(name_state)                                     AS name,
        argMaxMerge(project_id_state)                               AS project_id,
        argMaxMerge(client_id_state)                                AS client_id,
        argMaxMerge(user_properties_state)                          AS user_properties,
        JSONExtractString(argMaxMerge(user_properties_state), '%s') AS property,
        maxMerge(version_state)                                     AS version,
        argMaxMerge(updated_at_state)                               AS updated_at
    FROM customer_profiles_latest final
    GROUP BY nexora_id
    HAVING argMaxMerge(project_id_state) = '%s'
),
cp_resolved AS (
    SELECT
        identity_key,
        argMax(nexora_id, version)       AS nexora_id,
        max(id)                          AS customer_profile_id,
        argMax(external_user_id, version) AS external_user_id,
        argMax(email, version)           AS email,
        argMax(mobile, version)          AS mobile,
        argMax(name, version)            AS name,
        argMax(project_id, version)      AS project_id,
        argMax(client_id, version)       AS client_id,
        argMax(user_properties, version) AS user_properties,
        argMax(property, version)        AS property,
        max(updated_at)                  AS updated_at
    FROM cp_inner
    GROUP BY identity_key
)`, property, projectID)
}

// ─── per-group parsed result ─────────────────────────────────────────────────

type parsedGroup struct {
	eventFilterClauses [][]string
	eventMatchMode     string
	havingClauses      []string
	upMatchMode        string
	matchMode          string
}

// ─── build subquery for one group returning identity_keys ────────────────────

func buildGroupSubquery(pg parsedGroup, projectID, property string) string {
	hasEvents := len(pg.eventFilterClauses) > 0
	hasUserProps := len(pg.havingClauses) > 0

	if hasEvents && hasUserProps {
		return buildMixedGroupSubquery(pg, projectID, property)
	} else if hasEvents {
		return buildEventOnlyGroupSubquery(pg, projectID, property)
	} else {
		return buildUserPropOnlyGroupSubquery(pg, projectID, property)
	}
}

func buildEventOnlyGroupSubquery(pg parsedGroup, projectID, property string) string {
	eventSelects := []string{}
	for _, conditions := range pg.eventFilterClauses {
		whereClause := "WHERE " + strings.Join(conditions, " AND ")
		eventSelects = append(eventSelects, fmt.Sprintf(
			`SELECT DISTINCT ev.nexora_id
        FROM events ev
        INNER JOIN event_daily ed ON ev.event_name = ed.event_name AND ev.nexora_id = ed.nexora_id
        %s`, whereClause))
	}

	setOp := "INTERSECT"
	if strings.ToUpper(pg.eventMatchMode) == "OR" {
		setOp = "UNION DISTINCT"
	}
	eventBlock := strings.Join(eventSelects, "\n        "+setOp+"\n        ")

	return fmt.Sprintf(`(
    WITH
    %s,
    cp_filtered AS (
        SELECT identity_key, nexora_id FROM cp_resolved
    ),
    event_users AS (
        %s
    )
    SELECT cp.identity_key FROM event_users eu
    INNER JOIN cp_filtered cp ON eu.nexora_id = cp.nexora_id
)`, cpResolutionCTEs(projectID, property), eventBlock)
}

func buildUserPropOnlyGroupSubquery(pg parsedGroup, projectID, property string) string {
	whereClause := ""
	if len(pg.havingClauses) > 0 {
		whereClause = "WHERE " + strings.Join(pg.havingClauses, " "+strings.ToUpper(pg.upMatchMode)+" ")
	}

	return fmt.Sprintf(`(
    WITH
    %s
    SELECT identity_key FROM cp_resolved
    %s
)`, cpResolutionCTEs(projectID, property), whereClause)
}

func buildMixedGroupSubquery(pg parsedGroup, projectID, property string) string {
	eventSelects := []string{}
	for _, conditions := range pg.eventFilterClauses {
		whereClause := "WHERE " + strings.Join(conditions, " AND ")
		eventSelects = append(eventSelects, fmt.Sprintf(
			`SELECT DISTINCT ev.nexora_id
        FROM events ev
        INNER JOIN event_daily ed ON ev.event_name = ed.event_name AND ev.nexora_id = ed.nexora_id
        %s`, whereClause))
	}

	setOp := "INTERSECT"
	if strings.ToUpper(pg.eventMatchMode) == "OR" {
		setOp = "UNION DISTINCT"
	}
	eventBlock := strings.Join(eventSelects, "\n        "+setOp+"\n        ")

	whereClause := ""
	if len(pg.havingClauses) > 0 {
		whereClause = "WHERE " + strings.Join(pg.havingClauses, " "+strings.ToUpper(pg.upMatchMode)+" ")
	}

	return fmt.Sprintf(`(
    WITH
    %s,
    cp_filtered AS (
        SELECT * FROM cp_resolved
        %s
    ),
    event_users AS (
        %s
    )
    SELECT cp.identity_key FROM event_users eu
    INNER JOIN cp_filtered cp ON eu.nexora_id = cp.nexora_id
)`, cpResolutionCTEs(projectID, property), whereClause, eventBlock)
}

// ─── final SELECT ─────────────────────────────────────────────────────────────

func buildFinalSelect(combinedIdentityKeys string, projectID, property string, req models.SegmentNewPayload) map[string]string {
	selectColumns := []string{
		"customer_profile_id", "external_user_id", "nexora_id",
		"email", "mobile", "name", "project_id", "client_id",
		"user_properties", "property",
	}
	if req.Source != "campaign_service" {
		selectColumns = append(selectColumns, "updated_at")
	}

	limitAndOffsets := ""
	if req.LimitStatement != "" {
		limitAndOffsets = req.LimitStatement
	}
	fmt.Println(limitAndOffsets)
	fmt.Println("(((((((((((((((limitAndOffsets)))))))))))))))")
	selectStatement := fmt.Sprintf(`WITH
		%s,
		combined_identity_keys AS (
			%s
		)
		SELECT %s
		FROM cp_resolved
		WHERE identity_key IN (SELECT identity_key FROM combined_identity_keys)
		ORDER BY updated_at DESC NULLS LAST
		%s`,
		cpResolutionCTEs(projectID, property),
		combinedIdentityKeys,
		strings.Join(selectColumns, ", "),
		limitAndOffsets,
	)
	countSelectStatement := fmt.Sprintf(`WITH
		%s,
		combined_identity_keys AS (
			%s
		)
		SELECT %s
		FROM cp_resolved
		WHERE identity_key IN (SELECT identity_key FROM combined_identity_keys)`,
		cpResolutionCTEs(projectID, property),
		combinedIdentityKeys,
		strings.Join(selectColumns, ", "),
	)

	return map[string]string{
		"select": selectStatement,
		"count":  countSelectStatement,
	}
}

// ─── main entry point ─────────────────────────────────────────────────────────

func EvaluteRaw(req models.SegmentNewPayload) (map[string]interface{}, error) {
	groupSetOp := "INTERSECT"
	switch strings.ToUpper(req.GroupCondition) {
	case "OR":
		groupSetOp = "UNION DISTINCT"
	case "NOT":
		groupSetOp = "EXCEPT"
	}

	includeAnonymouseUsers := "no"
	groupSubqueries := []string{}

	for _, group := range req.Groups {
		pg := parsedGroup{
			matchMode: strings.ToUpper(group.MatchMode),
		}
		if pg.matchMode == "" {
			pg.matchMode = "AND"
		}

		eventFiltersMatchMode := strings.ToUpper(group.MatchMode)
		upFiltersMatchMode := strings.ToUpper(group.MatchMode)

		for _, filter := range group.Filters {
			switch filter.FilterCategory {

			case "event":
				var ec models.EventCondition
				if err := json.Unmarshal(filter.Condition, &ec); err != nil {
					fmt.Println("event unmarshal err:", err)
					continue
				}

				singleEventConditions := []string{}

				if ec.Query != nil {
					where := []string{}
					having := []string{}
					field := ""
					for _, r := range ec.Query.Rules {
						handleTypedRule(r, &where, &having, &field, includeAnonymouseUsers)
					}
					if len(where) > 0 {
						singleEventConditions = append(singleEventConditions,
							fmt.Sprintf("( %s )", strings.Join(where, " "+strings.ToUpper(ec.Query.Combinator)+" ")))
					}
				}

				if ec.Time != nil {
					if tc := getTimeConditionsTyped(ec.Time); tc != "" {
						singleEventConditions = append(singleEventConditions, tc)
					}
				}

				if ec.Count != nil {
					if cc := getCountConditionsTyped(ec.Count); cc != "" {
						singleEventConditions = append(singleEventConditions, cc)
					}
				}

				singleEventConditions = append(singleEventConditions,
					fmt.Sprintf("ed.event_name = '%s'", ec.EventName))

				pg.eventFilterClauses = append(pg.eventFilterClauses, singleEventConditions)
				pg.eventMatchMode = eventFiltersMatchMode

			case "user_property":
				var up models.UserPropertyCondition
				if err := json.Unmarshal(filter.Condition, &up); err != nil {
					fmt.Println("user_property unmarshal err:", err)
					continue
				}

				if up.UserPropertyQuery != nil {
					where := []string{}
					having := []string{}
					field := "user"
					for _, r := range up.UserPropertyQuery.Rules {
						handleTypedRule(r, &where, &having, &field, includeAnonymouseUsers)
					}
					if len(having) > 0 {
						pg.havingClauses = append(pg.havingClauses,
							fmt.Sprintf("( %s )", strings.Join(having, " "+strings.ToUpper(up.UserPropertyQuery.Combinator)+" ")))
					}
					pg.upMatchMode = upFiltersMatchMode
				}
			}
		}

		subquery := buildGroupSubquery(pg, req.ProjectID, req.Property)
		groupSubqueries = append(groupSubqueries, subquery)
	}

	// Combine all group subqueries — now using identity_key instead of nexora_id
	var combinedIdentityKeys string
	if len(groupSubqueries) == 1 {
		combinedIdentityKeys = fmt.Sprintf("SELECT identity_key FROM %s", groupSubqueries[0])
	} else {
		parts := []string{}
		for _, sq := range groupSubqueries {
			parts = append(parts, fmt.Sprintf("SELECT identity_key FROM %s", sq))
		}
		combinedIdentityKeys = strings.Join(parts, "\n    "+groupSetOp+"\n    ")
	}

	if len(req.NexoraIDs) > 0 {
		inList := buildInCondition("identity_key", req.NexoraIDs)
		combinedIdentityKeys = fmt.Sprintf(
			"SELECT identity_key FROM (%s) AS grp_combined WHERE 1=1 %s",
			combinedIdentityKeys, inList,
		)
	}

	overallSelectStatement := buildFinalSelect(combinedIdentityKeys, req.ProjectID, req.Property, req)

	countOverallStatement := fmt.Sprintf(
		"SELECT COUNT(*) AS total_count FROM (%s) AS count_base",
		overallSelectStatement["count"],
	)

	var count uint64
	if req.IsNeedCount {
		clientDBManager := db.NewClientDB()
		clickhouseConn, err := clientDBManager.GetCHDB(req.ClientID, req.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get clickhouse connection: %v", err)
		}
		row := clickhouseConn.QueryRow(context.Background(), countOverallStatement)
		if err := row.Scan(&count); err != nil {
			return nil, fmt.Errorf("failed to get count: %v", err)
		}
	}

	fmt.Println(groupSetOp)
	fmt.Println("(((((groupSetOp)))))")

	query := map[string]string{
		"overall_statement":       overallSelectStatement["select"],
		"count_overall_statement": countOverallStatement,
		"group_set_operator":      groupSetOp,
	}

	return map[string]interface{}{
		"query": query,
		"count": count,
	}, nil
}
