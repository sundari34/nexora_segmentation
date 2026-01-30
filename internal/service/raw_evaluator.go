package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nexora/nexora_segmentation/internal/models"
)

func getTimeConditionsTyped(tc *models.TimeCondition) string {
	if tc == nil {
		return ""
	}

	op := strings.ToLower(tc.Operator)
	now := time.Now().UTC()

	switch op {

	case "last_n_days":
		return fmt.Sprintf(
			"ed.event_date <= %s",
			chDateTime(now.AddDate(0, 0, -tc.Value)),
		)

	case "next_n_days":
		return fmt.Sprintf(
			"ed.event_date >= %s",
			chDateTime(now.AddDate(0, 0, tc.Value)),
		)

	case "on":
		return fmt.Sprintf(
			"ed.event_date = %s",
			chDateTime(now.AddDate(0, 0, tc.Value)),
		)

	case "before":
		return fmt.Sprintf(
			"ed.event_date < %s",
			chDateTime(now.AddDate(0, 0, tc.Value)),
		)

	case "after":
		return fmt.Sprintf(
			"ed.event_date > %s",
			chDateTime(now.AddDate(0, 0, tc.Value)),
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

	switch op {

	case "greater_than":
		return fmt.Sprintf("ed.event_count > %d", cc.Value)

	case "greater_than_or_equal":
		return fmt.Sprintf("ed.event_count >= %d", cc.Value)

	case "less_than":
		return fmt.Sprintf("ed.event_count < %d", cc.Value)

	case "less_than_or_equal":
		return fmt.Sprintf("ed.event_count <= %d", cc.Value)

	case "between":
		return fmt.Sprintf(
			"ed.event_count BETWEEN %d AND %d",
			cc.Min, cc.Max,
		)

	case "equal":
		return fmt.Sprintf("ed.event_count = %d", cc.Value)

	default:
		return ""
	}
}

func handleTypedRule(
	r models.Rule,
	where *[]string,
	scope *string,
) {
	op := getCHEquivalentOperator(r.Operator)

	// Nested rule (user / context)
	if nested, ok := r.Value.(models.QueryBlock); ok {
		*scope = r.Field
		for _, nr := range nested.Rules {
			handleTypedRule(nr, where, scope)
		}
		return
	}

	if *scope == "user" {
		cp := getCustomerProfileEquivalentField(r.Field)
		if op == "like" {
			*where = append(*where,
				fmt.Sprintf("%s %s '%%%v%%'", cp, op, r.Value),
			)
		} else {
			*where = append(*where,
				fmt.Sprintf("%s %s '%v'", cp, op, r.Value),
			)
		}
	} else {
		ev := getEventsEquivalentField(r.Field)
		if op == "like" {
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
	default:
		return ""
	}
}

func getCustomerProfileEquivalentField(field string) string {
	staticColumns := map[string]bool{
		"id": true, "email": true, "mobile": true, "name": true,
		"external_user_id": true, "client_id": true,
		"project_id": true, "created_at": true, "updated_at": true,
	}

	if staticColumns[field] {
		return fmt.Sprintf("cp.%s", field)
	}
	return fmt.Sprintf("JSONExtractString(cp.user_properties, '%s')", field)
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
		t.UTC().Format("2006-01-02 15:04:05"),
	)
}

func EvaluteRaw(req models.SegmentNewPayload) ([]models.Member, error) {
	groupCondition := strings.ToUpper(req.GroupCondition)
	if groupCondition == "" {
		groupCondition = "AND"
	}

	groupRules := []string{}

	isOnlyUserProperty := true

	for _, group := range req.Groups {
		filterClauses := []string{}

		for _, filter := range group.Filters {

			switch filter.FilterCategory {

			// ---------- EVENT ----------
			case "event":
				var ec models.EventCondition
				if err := json.Unmarshal(filter.Condition, &ec); err != nil {
					continue
				}

				// query rules
				if ec.Query != nil {
					where := []string{}
					field := ""
					for _, r := range ec.Query.Rules {
						handleTypedRule(r, &where, &field)
					}
					filterClauses = append(
						filterClauses,
						fmt.Sprintf("( %s )",
							strings.Join(where, " "+strings.ToUpper(ec.Query.Combinator)+" "),
						),
					)
				}

				// time
				if ec.Time != nil {
					filterClauses = append(filterClauses, getTimeConditionsTyped(ec.Time))
				}

				// count
				if ec.Count != nil {
					filterClauses = append(filterClauses, getCountConditionsTyped(ec.Count))
				}

				// event name
				filterClauses = append(
					filterClauses,
					fmt.Sprintf("ed.event_name = '%s'", ec.EventName),
				)

				isOnlyUserProperty = false

			// ---------- USER PROPERTY ----------
			case "user_property":
				var up models.UserPropertyCondition
				if err := json.Unmarshal(filter.Condition, &up); err != nil {
					continue
				}

				if up.UserPropertyQuery != nil {
					where := []string{}
					field := "user"
					for _, r := range up.UserPropertyQuery.Rules {
						handleTypedRule(r, &where, &field)
					}
					filterClauses = append(
						filterClauses,
						fmt.Sprintf("( %s )",
							strings.Join(where, " "+strings.ToUpper(up.UserPropertyQuery.Combinator)+" "),
						),
					)
				}
			}
		}

		groupRules = append(
			groupRules,
			strings.Join(filterClauses, " "+strings.ToUpper(group.MatchMode)+" "),
		)
	}

	finalWhere := strings.Join(groupRules, " "+groupCondition+" ")
	fmt.Println("********* QUERY MAP *********")
	joinStatement := "customer_profiles AS cp INNER JOIN nexora_profiles AS np ON cp.id = np.customer_profile_id INNER JOIN events AS ev ON np.nexora_id = ev.nexora_id INNER JOIN event_daily AS ed ON ev.event_name = ed.event_name"
	if isOnlyUserProperty {
		joinStatement = "customer_profiles AS cp INNER JOIN nexora_profiles AS np ON cp.id = np.customer_profile_id"
	}
	fmt.Println(map[string]string{
		"where_statement": finalWhere,
		"join_statement":  joinStatement,
	})

	FinalQuery := fmt.Sprintf("SELECT cp.email, cp.mobile, cp.id FROM %s WHERE %s", joinStatement, finalWhere)
	fmt.Println(FinalQuery)
	fmt.Println("(((FinalQuery)))")
	return nil, nil
}
