package utils

import (
	"fmt"
	"strings"
	"time"
)

// DeriveDateRange calculates the date range based on operator and inputs
func DeriveDateRange(operator string, start, end, value, dayValue, dayCountValue *FlexibleString, now time.Time, loc *time.Location) (string, string, error) {
	var startDate, endDate time.Time
	var err error

	// Parse helper
	parseDate := func(fs *FlexibleString) (time.Time, error) {
		if fs == nil || fs.IsEmpty() {
			return time.Time{}, fmt.Errorf("empty date string")
		}
		return time.ParseInLocation("2006-01-02", fs.String(), loc)
	}

	switch strings.ToLower(operator) {
	case "between":
		startDate, err = parseDate(start)
		if err != nil {
			return "", "", err
		}
		endDate, err = parseDate(end)
		if err != nil {
			return "", "", err
		}

	case "on":
		startDate, err = parseDate(value)
		if err != nil {
			return "", "", err
		}
		endDate = startDate

	case "before":
		//endDate, err = parseDate(value)

		v := value
		if (v == nil || v.IsEmpty()) && dayValue != nil {
			v = dayValue
		}
		endDate, err = parseDate(v)
		if err != nil {
			return "", "", err
		}
		startDate = time.Date(1970, 1, 1, 0, 0, 0, 0, loc)

	case "after":
		v := value
		if (v == nil || v.IsEmpty()) && dayValue != nil {
			v = dayValue
		}
		startDate, err = parseDate(v)
		if err != nil {
			return "", "", err
		}
		endDate = now

	case "last_n_days":
		var days int
		if dayCountValue != nil && !dayCountValue.IsEmpty() {
			days, err = dayCountValue.ToInt()
			if err != nil {
				return "", "", err
			}
		} else if value != nil && !value.IsEmpty() {
			days, err = value.ToInt()
			if err != nil {
				return "", "", err
			}
		} else {
			return "", "", fmt.Errorf("time.day_count_value or time.value required for last_n_days")
		}

		endDate = now
		startDate = now.AddDate(0, 0, -days)

	default:
		return "", "", fmt.Errorf("unsupported operator: %s", operator)
	}

	return startDate.Format("2006-01-02"), endDate.Format("2006-01-02"), nil
}

// // local struct to avoid import cycle with models
// type UserPropertyRule struct {
// 	ID          string `json:"id"`
// 	Field       string `json:"field"`
// 	Operator    string `json:"operator"`
// 	ValueSource string `json:"valueSource"`
// 	Value       any    `json:"value"`
// }

// type UserPropertyQueryLite struct {
// 	ID         string             `json:"id"`
// 	Rules      []UserPropertyRule `json:"rules"`
// 	Combinator string             `json:"combinator"`
// 	Not        bool               `json:"not"`
// }

// func BuildUserPropertyQuery(upq *UserPropertyQueryLite) (string, []any, error) {
// 	var clauses []string
// 	var params []any

// 	for _, rule := range upq.Rules {
// 		op := mapSQLOperator(rule.Operator)
// 		if op == "" {
// 			return "", nil, fmt.Errorf("unsupported operator: %s", rule.Operator)
// 		}

// 		clauses = append(clauses, fmt.Sprintf("%s %s ?", rule.Field, op))
// 		params = append(params, rule.Value)
// 	}

// 	combinator := strings.ToUpper(upq.Combinator)
// 	if combinator != "AND" && combinator != "OR" {
// 		combinator = "AND"
// 	}

// 	whereClause := "(" + strings.Join(clauses, " "+combinator+" ") + ")"
// 	if upq.Not {
// 		whereClause = "NOT " + whereClause
// 	}

// 	return whereClause, params, nil
// }

// func mapSQLOperator(op string) string {
// 	switch strings.ToLower(op) {
// 	case "=", "eq":
// 		return "="
// 	case "!=":
// 		return "!="
// 	case ">", "gt":
// 		return ">"
// 	case "<", "lt":
// 		return "<"
// 	case ">=", "gte":
// 		return ">="
// 	case "<=", "lte":
// 		return "<="
// 	case "like":
// 		return "LIKE"
// 	default:
// 		return ""
// 	}
// }

// UserPropertyRule defines a single condition in the query.
type UserPropertyRule struct {
	ID          string `json:"id"`
	Field       string `json:"field"`
	Operator    string `json:"operator"`
	ValueSource string `json:"valueSource"`
	Value       any    `json:"value"`
}

// UserPropertyQueryLite groups multiple rules with a combinator.
type UserPropertyQueryLite struct {
	ID         string             `json:"id"`
	Rules      []UserPropertyRule `json:"rules"`
	Combinator string             `json:"combinator"`
	Not        bool               `json:"not"`
}

// BuildUserPropertyQuery builds a WHERE clause for MySQL queries.
// It supports JSON fields (for `additonal_properties`) and plain columns.
func BuildUserPropertyQuery(upq *UserPropertyQueryLite) (string, []any, error) {
	var clauses []string
	var params []any

	for _, rule := range upq.Rules {
		op := strings.ToLower(strings.TrimSpace(rule.Operator))
		fieldExpr := buildFieldExpression(rule.Field)

		switch op {
		// ✅ Handle NOT NULL / IS NULL first (no params)
		case "notnull":
			clauses = append(clauses, fmt.Sprintf("(%s IS NOT NULL AND %s != '')", fieldExpr, fieldExpr))
			continue

		case "isnull":
			clauses = append(clauses, fmt.Sprintf("(%s IS NULL OR %s = '')", fieldExpr, fieldExpr))
			continue

		default:
			sqlOp := mapSQLOperator(op)
			if sqlOp == "" {
				return "", nil, fmt.Errorf("unsupported operator: %s", rule.Operator)
			}

			// Handle value formatting (e.g. LIKE)
			val := rule.Value
			if strings.ToLower(sqlOp) == "like" {
				val = fmt.Sprintf("%%%v%%", rule.Value)
			}

			clauses = append(clauses, fmt.Sprintf("%s %s ?", fieldExpr, sqlOp))
			params = append(params, val)
		}
	}

	combinator := strings.ToUpper(upq.Combinator)
	if combinator != "AND" && combinator != "OR" {
		combinator = "AND"
	}

	whereClause := "(" + strings.Join(clauses, " "+combinator+" ") + ")"
	if upq.Not {
		whereClause = "NOT " + whereClause
	}

	return whereClause, params, nil
}

// mapSQLOperator translates logical operators to SQL-safe ones.
func mapSQLOperator(op string) string {
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
		return "LIKE"
	default:
		return ""
	}
}

func buildFieldExpression(field string) string {
	// List of static SQL columns in customer_profiles table
	staticColumns := map[string]bool{
		"id": true, "email": true, "mobile": true, "name": true,
		"external_user_id": true, "client_id": true, "project_id": true,
		"created_at": true, "updated_at": true,
	}

	if staticColumns[field] {
		return fmt.Sprintf("cp.%s", field)
	}

	// Dynamic field → stored in JSON column "user_property"
	// Allow field like "age", "signup_source" without prefix
	return fmt.Sprintf("JSON_UNQUOTE(JSON_EXTRACT(cp.user_properties, '$.%s'))", field)
}
