package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

// We support minimal subset to start, extend as needed.
type rqbRoot struct {
	ID         string    `json:"id"`
	Rules      []rqbRule `json:"rules"`
	Combinator string    `json:"combinator"` // "and"/"or"
	Not        bool      `json:"not"`
}

type rqbRule struct {
	ID          string `json:"id"`
	Field       string `json:"field"`    // e.g., "app"
	Operator    string `json:"operator"` // "=" etc.
	ValueSource string `json:"valueSource"`
	Value       any    `json:"value"` // could be scalar or group
	Combinator  string `json:"combinator"`
	Not         bool   `json:"not"`
	Match       *struct {
		Mode      string `json:"mode"`
		Threshold int    `json:"threshold"`
	} `json:"match"`
}

// Very small mapper: supports "app.build_number" == "<num>" → column "app_build_number"
func buildWhereFromRQB(rqbJSON string) (string, []any, error) {
	var root rqbRoot
	if err := json.Unmarshal([]byte(rqbJSON), &root); err != nil {
		return "", nil, fmt.Errorf("invalid RQB JSON: %w", err)
	}

	var clauses []string
	var args []any

	var walk func(rule rqbRule) error
	walk = func(rule rqbRule) error {
		// 2 cases: rule with nested value (group) OR simple field
		switch v := rule.Value.(type) {
		case map[string]any:
			// Nested group, search inside "rules"
			if subRules, ok := v["rules"].([]any); ok {
				var subClauses []string
				var subArgs []any
				for _, sr := range subRules {
					m := sr.(map[string]any)
					r := rqbRule{
						Field:    getString(m, "field"),
						Operator: getString(m, "operator"),
						Value:    m["value"],
					}
					where, wargs, err := simpleFieldToClause(r)
					if err != nil {
						return err
					}
					if where != "" {
						subClauses = append(subClauses, where)
						subArgs = append(subArgs, wargs...)
					}
				}
				if len(subClauses) > 0 {
					comb := strings.ToUpper(getString(v, "combinator"))
					if comb != "AND" && comb != "OR" {
						comb = "AND"
					}
					clauses = append(clauses, "("+strings.Join(subClauses, " "+comb+" ")+")")
					args = append(args, subArgs...)
				}
			}
		default:
			// simple clause
			where, wargs, err := simpleFieldToClause(rule)
			if err != nil {
				return err
			}
			if where != "" {
				clauses = append(clauses, where)
				args = append(args, wargs...)
			}
		}
		return nil
	}

	for _, r := range root.Rules {
		if err := walk(r); err != nil {
			return "", nil, err
		}
	}

	comb := strings.ToUpper(root.Combinator)
	if comb != "AND" && comb != "OR" {
		comb = "AND"
	}

	if len(clauses) == 0 {
		return "", nil, nil
	}
	return "(" + strings.Join(clauses, " "+comb+" ") + ")", args, nil
}

func simpleFieldToClause(r rqbRule) (string, []any, error) {
	fieldPath := r.Field
	op := r.Operator
	val := r.Value

	// Map supported paths → column names
	switch strings.ToLower(fieldPath) {
	case "app.build_number":
		col := "app_build_number"
		switch op {
		case "=":
			return fmt.Sprintf("%s = ?", col), []any{toString(val)}, nil
		case "!=":
			return fmt.Sprintf("%s != ?", col), []any{toString(val)}, nil
		default:
			return "", nil, fmt.Errorf("unsupported operator for %s: %s", fieldPath, op)
		}
	default:
		// Unsupported fields are ignored for now
		return "", nil, nil
	}
}

func getString(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// JSON numbers decode as float64
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.f", t), "0"), ".")
	default:
		return fmt.Sprintf("%v", v)
	}
}
