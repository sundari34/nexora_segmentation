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

func chDate(t time.Time) string {
	return fmt.Sprintf("toDate('%s', 'UTC')", t.UTC().Format("2006-01-02"))
}

func chDateTime(t time.Time) string {
	return fmt.Sprintf("toDateTime('%s', 'UTC')", t.UTC().Format("2006-01-02"))
}

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
		from := now.AddDate(0, 0, -days)
		return fmt.Sprintf(
			"ed.event_date >= toDate('%s', 'UTC') AND ed.event_date <= toDate('%s', 'UTC')",
			from.UTC().Format("2006-01-02"),
			now.UTC().Format("2006-01-02"),
		)
	case "next_n_days":
		to := now.AddDate(0, 0, days)
		return fmt.Sprintf(
			"ed.event_date >= toDate('%s', 'UTC') AND ed.event_date <= toDate('%s', 'UTC')",
			now.UTC().Format("2006-01-02"),
			to.UTC().Format("2006-01-02"),
		)
	case "on":
		d, err := time.Parse("2006-01-02", fmt.Sprintf("%v", tc.Value))
		if err != nil {
			return ""
		}
		return fmt.Sprintf("ed.event_date = toDate('%s', 'UTC')", d.UTC().Format("2006-01-02"))
	case "before":
		d, err := time.Parse("2006-01-02", fmt.Sprintf("%v", tc.Value))
		if err != nil {
			return ""
		}
		return fmt.Sprintf("ed.event_date < toDate('%s', 'UTC')", d.UTC().Format("2006-01-02"))
	case "after":
		d, err := time.Parse("2006-01-02", fmt.Sprintf("%v", tc.Value))
		if err != nil {
			return ""
		}
		return fmt.Sprintf("ed.event_date > toDate('%s', 'UTC')", d.UTC().Format("2006-01-02"))
	case "between":
		start, err1 := time.Parse("2006-01-02", tc.StartDate)
		end, err2 := time.Parse("2006-01-02", tc.EndDate)
		if err1 != nil || err2 != nil {
			return ""
		}
		return fmt.Sprintf(
			"ed.event_date >= toDate('%s', 'UTC') AND ed.event_date <= toDate('%s', 'UTC')",
			start.UTC().Format("2006-01-02"),
			end.UTC().Format("2006-01-02"),
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

	var fieldName string
	var targetSlice *[]string

	if *scope == "user" {
		fieldName = getCustomerProfileEquivalentField(r.Field)
		targetSlice = having
	} else {
		fieldName = getEventsEquivalentField(r.Field)
		targetSlice = where
	}

	opLower := strings.ToLower(op)
	var condition string

	switch opLower {
	case "last_n_days", "next_n_days", "on", "before", "after", "between":
		now := time.Now().UTC()
		switch opLower {
		case "last_n_days":
			var days int
			if v, ok := r.Value.(float64); ok {
				days = int(v)
			}
			from := now.AddDate(0, 0, -days)
			condition = fmt.Sprintf("%s >= toDate('%s', 'UTC') AND %s <= toDate('%s', 'UTC')",
				fieldName, from.Format("2006-01-02"), fieldName, now.Format("2006-01-02"))
		case "next_n_days":
			var days int
			if v, ok := r.Value.(float64); ok {
				days = int(v)
			}
			to := now.AddDate(0, 0, days)
			condition = fmt.Sprintf("%s >= toDate('%s', 'UTC') AND %s <= toDate('%s', 'UTC')",
				fieldName, now.Format("2006-01-02"), fieldName, to.Format("2006-01-02"))
		case "on":
			d, _ := time.Parse("2006-01-02", fmt.Sprintf("%v", r.Value))
			condition = fmt.Sprintf("%s = toDate('%s', 'UTC')", fieldName, d.Format("2006-01-02"))
		case "before":
			d, _ := time.Parse("2006-01-02", fmt.Sprintf("%v", r.Value))
			condition = fmt.Sprintf("%s < toDate('%s', 'UTC')", fieldName, d.Format("2006-01-02"))
		case "after":
			d, _ := time.Parse("2006-01-02", fmt.Sprintf("%v", r.Value))
			condition = fmt.Sprintf("%s > toDate('%s', 'UTC')", fieldName, d.Format("2006-01-02"))
		case "between":
			var start, end string
			valStr := fmt.Sprintf("%v", r.Value)
			parts := strings.Fields(strings.Trim(valStr, "[]"))
			if len(parts) == 2 {
				start = parts[0]
				end = parts[1]
			}
			if start != "" && end != "" {
				if r.Type == "date" {
					condition = fmt.Sprintf("toDate(%s) >= toDate('%s', 'UTC') AND toDate(%s) <= toDate('%s', 'UTC')",
						fieldName, start, fieldName, end)
				} else {
					condition = fmt.Sprintf("%s >= '%s' AND %s <= '%s'", fieldName, start, fieldName, end)
				}
			}
		}
	case "is null", "is not null":
		if opLower == "is not null" {
			condition = fmt.Sprintf("(%s IS NOT NULL AND %s != '' AND %s != '0' AND LOWER(%s) != 'none')", fieldName, fieldName, fieldName, fieldName)
		} else {
			condition = fmt.Sprintf("(%s IS NULL OR %s = '' OR %s = '0' OR LOWER(%s) = 'none')", fieldName, fieldName, fieldName, fieldName)
		}
	case "like", "not like", "beginswith", "endswith", "doesnotbeginwith", "doesnotendwith":
		columnExpr := fmt.Sprintf("LOWER(%s)", fieldName)
		valLower := strings.ToLower(fmt.Sprintf("%v", r.Value))
		actualOp := op
		if strings.Contains(opLower, "begin") || strings.Contains(opLower, "end") {
			if strings.Contains(opLower, "not") || strings.Contains(opLower, "doesnot") {
				actualOp = "NOT LIKE"
			} else {
				actualOp = "LIKE"
			}
		}
		switch opLower {
		case "beginswith", "doesnotendwith":
			condition = fmt.Sprintf("%s %s '%s%%'", columnExpr, actualOp, valLower)
		case "endswith", "doesnotbeginwith":
			condition = fmt.Sprintf("%s %s '%%%s'", columnExpr, actualOp, valLower)
		default:
			condition = fmt.Sprintf("%s %s '%%%s%%'", columnExpr, actualOp, valLower)
		}
	case "in", "not in":
		var valuesArr []string
		switch v := r.Value.(type) {
		case []string:
			for _, val := range v {
				valuesArr = append(valuesArr, fmt.Sprintf("LOWER('%v')", val))
			}
		default:
			valuesArr = []string{fmt.Sprintf("LOWER('%v')", v)}
		}
		condition = fmt.Sprintf("LOWER(%s) %s (%v)", fieldName, op, strings.Join(valuesArr, ", "))
	case ">", "<", ">=", "<=":
		condition = fmt.Sprintf("%s %s %v", fieldName, op, r.Value)
	default:
		condition = fmt.Sprintf("LOWER(%s) %s LOWER('%v')", fieldName, op, r.Value)
	}

	if condition != "" {
		*targetSlice = append(*targetSlice, condition)
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
		return strings.ToLower(op)
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
		"context_locale": true, "context_timezone": true, "user_id": true,
	}
	if directFields[field] {
		return fmt.Sprintf("ev.%s", field)
	}
	jsonPathMap := map[string]string{
		"sdk_version": "app.sdk_version", "app_version": "app.version",
		"app_build_number": "app.build_number", "context_locale": "context.locale",
		"context_timezone": "context.timezone",
	}
	if path, ok := jsonPathMap[field]; ok {
		return fmt.Sprintf("JSONExtractString(arrayElement(JSONExtract(ev.raw_payload, 'Array(JSON)'), 1), '%s')", path)
	}
	return fmt.Sprintf("JSONExtractString(ev.event_properties, '%s')", field)
}

// ─── cp resolution CTE ───────────────────────────────────────────────────────

func cpResolutionCTEs(projectID, property string) string {
	// identity_key logic: if external_user_id is set and not 'none', use it. Else use nexora_id.
	return fmt.Sprintf(`cp_inner AS (
    SELECT
        nexora_id,
        multiIf(
            argMaxMerge(external_user_id_state) != 'none' AND argMaxMerge(external_user_id_state) != '',
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

type parsedGroup struct {
	eventFilterClauses [][]string
	eventMatchMode     string
	havingClauses      []string
	upMatchMode        string
	matchMode          string
}

func buildGroupSubquery(pg parsedGroup, projectID, property string) string {
	if len(pg.eventFilterClauses) > 0 && len(pg.havingClauses) > 0 {
		return buildMixedGroupSubquery(pg, projectID, property)
	} else if len(pg.eventFilterClauses) > 0 {
		return buildEventOnlyGroupSubquery(pg, projectID, property)
	} else {
		return buildUserPropOnlyGroupSubquery(pg, projectID, property)
	}
}

func buildEventOnlyGroupSubquery(pg parsedGroup, projectID, property string) string {
	eventSelects := []string{}
	for _, conditions := range pg.eventFilterClauses {
		var currentEventName string
		hasNegativeCondition := false
		for _, c := range conditions {
			if strings.Contains(c, "!=") {
				hasNegativeCondition = true
				parts := strings.Split(c, "!=")
				if len(parts) > 1 {
					currentEventName = strings.TrimSpace(parts[1])
				}
				break
			}
		}

		if hasNegativeCondition && currentEventName != "" {
			query := fmt.Sprintf(`
            SELECT DISTINCT multiIf(user_id != 'none' AND user_id != '', user_id, nexora_id) AS identity_key
            FROM event_daily
            WHERE event_date < toDate('2026-04-29', 'UTC')
            EXCEPT
            SELECT multiIf(user_id != 'none' AND user_id != '', user_id, nexora_id) AS identity_key 
            FROM event_daily
            WHERE event_date < toDate('2026-04-29', 'UTC')
              AND event_name = %s`, currentEventName)
			eventSelects = append(eventSelects, query)
		} else {
			whereClause := "WHERE " + strings.Join(conditions, " AND ")
			eventSelects = append(eventSelects, fmt.Sprintf(
				`SELECT DISTINCT multiIf(ev.user_id != 'none' AND ev.user_id != '', ev.user_id, ev.nexora_id) AS identity_key
            FROM events ev
            INNER JOIN event_daily ed ON ev.event_name = ed.event_name AND ev.nexora_id = ed.nexora_id
            %s`, whereClause))
		}
	}

	setOp := "INTERSECT"
	if strings.ToUpper(pg.eventMatchMode) == "OR" {
		setOp = "UNION DISTINCT"
	}
	eventBlock := strings.Join(eventSelects, "\n        "+setOp+"\n        ")

	return fmt.Sprintf(`(
    WITH
    %s,
    event_identity_keys AS (
        %s
    )
    SELECT cp.identity_key, cp.external_user_id, cp.nexora_id 
    FROM event_identity_keys ek
    INNER JOIN cp_resolved cp ON ek.identity_key = cp.identity_key
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
    SELECT identity_key, external_user_id, nexora_id 
    FROM cp_resolved
    %s
)`, cpResolutionCTEs(projectID, property), whereClause)
}

func buildMixedGroupSubquery(pg parsedGroup, projectID, property string) string {
	eventSelects := []string{}
	for _, conditions := range pg.eventFilterClauses {
		var currentEventName string
		hasNegativeCondition := false
		for _, c := range conditions {
			if strings.Contains(c, "!=") {
				hasNegativeCondition = true
				parts := strings.Split(c, "!=")
				if len(parts) > 1 {
					currentEventName = strings.TrimSpace(parts[1])
				}
				break
			}
		}

		if hasNegativeCondition && currentEventName != "" {
			query := fmt.Sprintf(`
            SELECT DISTINCT multiIf(user_id != 'none' AND user_id != '', user_id, nexora_id) AS identity_key
            FROM event_daily
            WHERE event_date < toDate('2026-04-29', 'UTC')
            EXCEPT
            SELECT multiIf(user_id != 'none' AND user_id != '', user_id, nexora_id) AS identity_key 
            FROM event_daily
            WHERE event_date < toDate('2026-04-29', 'UTC')
              AND event_name = %s`, currentEventName)
			eventSelects = append(eventSelects, query)
		} else {
			whereClause := "WHERE " + strings.Join(conditions, " AND ")
			eventSelects = append(eventSelects, fmt.Sprintf(
				`SELECT DISTINCT multiIf(ev.user_id != 'none' AND ev.user_id != '', ev.user_id, ev.nexora_id) AS identity_key
            FROM events ev
            INNER JOIN event_daily ed ON ev.event_name = ed.event_name AND ev.nexora_id = ed.nexora_id
            %s`, whereClause))
		}
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
    event_identity_keys AS (
        %s
    )
    SELECT cp.identity_key, cp.external_user_id, cp.nexora_id 
    FROM event_identity_keys ek
    INNER JOIN cp_filtered cp ON ek.identity_key = cp.identity_key
)`, cpResolutionCTEs(projectID, property), whereClause, eventBlock)
}

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

	selectStatement := fmt.Sprintf(`WITH
		%s,
		combined_identity_keys AS (
			%s
		)
		SELECT %s
		FROM cp_resolved
		WHERE identity_key IN (SELECT identity_key FROM combined_identity_keys)
		ORDER BY nexora_id DESC, updated_at DESC NULLS LAST
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

func buildExternalFilterCondition(ef models.ExternalFilterStruct) string {
	if ef.SearchValue == "" {
		return ""
	}
	val := strings.TrimSpace(ef.SearchValue)
	if val == "" {
		return ""
	}

	switch strings.ToLower(ef.SearchType) {
	case "email":
		return fmt.Sprintf("email LIKE '%%%s%%'", val)
	case "mobile":
		return fmt.Sprintf("mobile LIKE '%%%s%%'", val)
	case "name":
		return fmt.Sprintf("name LIKE '%%%s%%'", val)
	case "nexora_id":
		return fmt.Sprintf("nexora_id = '%s'", val)
	case "external_user_id":
		return fmt.Sprintf("external_user_id = '%s'", val)
	case "identity_key":
		return fmt.Sprintf("identity_key = '%s'", val)
	default:
		return fmt.Sprintf(
			"(email LIKE '%%%s%%' OR mobile LIKE '%%%s%%' OR name LIKE '%%%s%%' OR nexora_id = '%s' OR external_user_id = '%s')",
			val, val, val, val, val,
		)
	}
}

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
		pg := parsedGroup{matchMode: strings.ToUpper(group.MatchMode)}
		if pg.matchMode == "" {
			pg.matchMode = "AND"
		}
		eventFiltersMatchMode := pg.matchMode
		upFiltersMatchMode := pg.matchMode

		for _, filter := range group.Filters {
			switch filter.FilterCategory {
			case "event":
				var ec models.EventCondition
				if err := json.Unmarshal(filter.Condition, &ec); err != nil {
					continue
				}
				singleEventConditions := []string{}
				if ec.Query != nil {
					where, having, field := []string{}, []string{}, ""
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
				if ec.Condition == "has_performed" {
					singleEventConditions = append(singleEventConditions, fmt.Sprintf("ed.event_name = '%s'", ec.EventName))
				} else {
					singleEventConditions = append(singleEventConditions, fmt.Sprintf("ed.event_name != '%s'", ec.EventName))
				}
				pg.eventFilterClauses = append(pg.eventFilterClauses, singleEventConditions)
				pg.eventMatchMode = eventFiltersMatchMode
			case "user_property":
				var up models.UserPropertyCondition
				if err := json.Unmarshal(filter.Condition, &up); err != nil {
					continue
				}
				if up.UserPropertyQuery != nil {
					where, having, field := []string{}, []string{}, "user"
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

	externalFilterCond := buildExternalFilterCondition(req.ExternalFilters)
	var combinedIdentityKeys string

	if len(groupSubqueries) == 0 {
		conditions := []string{"1=1"}
		if len(req.NexoraIDs) > 0 {
			escaped := []string{}
			for _, v := range req.NexoraIDs {
				v = strings.TrimSpace(v)
				if v != "" {
					escaped = append(escaped, fmt.Sprintf("'%s'", v))
				}
			}
			if len(escaped) > 0 {
				inList := strings.Join(escaped, ",")
				conditions = append(conditions, fmt.Sprintf("(nexora_id IN (%s) OR identity_key IN (%s))", inList, inList))
			}
		}
		if externalFilterCond != "" {
			conditions = append(conditions, externalFilterCond)
		}
		combinedIdentityKeys = fmt.Sprintf("SELECT identity_key FROM cp_resolved WHERE %s", strings.Join(conditions, " AND "))
	} else if len(groupSubqueries) == 1 {
		combinedIdentityKeys = fmt.Sprintf("SELECT identity_key FROM %s AS grp_0", groupSubqueries[0])
		wrapConditions := []string{"1=1"}
		if len(req.NexoraIDs) > 0 {
			escaped := []string{}
			for _, v := range req.NexoraIDs {
				v = strings.TrimSpace(v)
				if v != "" {
					escaped = append(escaped, fmt.Sprintf("'%s'", v))
				}
			}
			if len(escaped) > 0 {
				inList := strings.Join(escaped, ",")
				wrapConditions = append(wrapConditions, fmt.Sprintf("(nexora_id IN (%s) OR identity_key IN (%s))", inList, inList))
			}
		}
		if externalFilterCond != "" {
			wrapConditions = append(wrapConditions, externalFilterCond)
		}
		if len(wrapConditions) > 1 {
			combinedIdentityKeys = fmt.Sprintf(
				`SELECT identity_key FROM (
                WITH %s
                SELECT cp.* FROM cp_resolved AS cp
                INNER JOIN (%s) AS grp ON cp.identity_key = grp.identity_key
            ) AS grp_filtered WHERE %s`,
				cpResolutionCTEs(req.ProjectID, req.Property),
				combinedIdentityKeys,
				strings.Join(wrapConditions, " AND "),
			)
		}
	} else {
		parts := []string{}
		for i, sq := range groupSubqueries {
			parts = append(parts, fmt.Sprintf("SELECT identity_key FROM %s AS grp_%d", sq, i))
		}
		combinedIdentityKeys = strings.Join(parts, "\n    "+groupSetOp+"\n    ")
		wrapConditions := []string{"1=1"}
		if len(req.NexoraIDs) > 0 {
			escaped := []string{}
			for _, v := range req.NexoraIDs {
				v = strings.TrimSpace(v)
				if v != "" {
					escaped = append(escaped, fmt.Sprintf("'%s'", v))
				}
			}
			if len(escaped) > 0 {
				inList := strings.Join(escaped, ",")
				wrapConditions = append(wrapConditions, fmt.Sprintf("(nexora_id IN (%s) OR identity_key IN (%s))", inList, inList))
			}
		}
		if externalFilterCond != "" {
			wrapConditions = append(wrapConditions, externalFilterCond)
		}
		if len(wrapConditions) > 1 {
			combinedIdentityKeys = fmt.Sprintf(
				`SELECT identity_key FROM (
                WITH %s
                SELECT cp.* FROM cp_resolved AS cp
                INNER JOIN (%s) AS grp ON cp.identity_key = grp.identity_key
            ) AS grp_filtered WHERE %s`,
				cpResolutionCTEs(req.ProjectID, req.Property),
				combinedIdentityKeys,
				strings.Join(wrapConditions, " AND "),
			)
		}
	}

	overallSelectStatement := buildFinalSelect(combinedIdentityKeys, req.ProjectID, req.Property, req)
	countOverallStatement := fmt.Sprintf("SELECT COUNT(*) AS total_count FROM (%s) AS count_base", overallSelectStatement["count"])

	var count uint64
	if req.IsNeedCount {
		clientDBManager := db.NewClientDB()
		clickhouseConn, err := clientDBManager.GetCHDB(req.ClientID, req.ProjectID)
		if err != nil {
			return nil, err
		}
		row := clickhouseConn.QueryRow(context.Background(), countOverallStatement)
		if err := row.Scan(&count); err != nil {
			return nil, err
		}
	}

	query := map[string]string{
		"overall_statement":       overallSelectStatement["select"],
		"count_overall_statement": countOverallStatement,
		"group_set_operator":      groupSetOp,
	}

	return map[string]interface{}{"query": query, "count": count}, nil
}
