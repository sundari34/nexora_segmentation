package models

import "encoding/json"

type SegmentNewPayload struct {
	GroupCondition  string               `json:"group_condition,omitempty"`
	Groups          []RuleGroupRaw       `json:"groups,omitempty"`
	Property        string               `json:"property,omitempty"`
	Channel         string               `json:"channel,omitempty"`
	NexoraIDs       []string             `json:"nexora_id,omitempty"`
	ClientID        string               `json:"client_id,omitempty"`
	ProjectID       string               `json:"project_id,omitempty"`
	IsNeedCount     bool                 `json:"need_count,omitempty"`
	IsNeedSQL       bool                 `json:"need_sql,omitempty"`
	IsNeedValue     bool                 `json:"need_value,omitempty"`
	Source          string               `json:"source"`
	LimitStatement  string               `json:"limit_statement"`
	ExternalFilters ExternalFilterStruct `json:"external_filters,omitempty"`
}

type ExternalFilterStruct struct {
	SearchValue string `json:"search_value"`
	SearchType  string `json:"search_type"`
}

type RuleGroupRaw struct {
	MatchMode string      `json:"matchMode"`
	Filters   []FilterRaw `json:"filters"`
}

type FilterRaw struct {
	FilterCategory string          `json:"filter_category"`
	Condition      json.RawMessage `json:"condition"`
}

type EventCondition struct {
	Time      *TimeCondition  `json:"time,omitempty"`
	Count     *CountCondition `json:"count,omitempty"`
	Query     *QueryBlock     `json:"query,omitempty"`
	EventName string          `json:"event_name"`
	Condition string          `json:"condition"` // has_performed, has not performed
}

type UserPropertyCondition struct {
	UserPropertyQuery *QueryBlock `json:"user_property_query"`
}

type QueryBlock struct {
	Combinator string `json:"combinator"`
	Rules      []Rule `json:"rules"`
}

type Rule struct {
	Type     string `json:"type"`     // dynamic values from DB
	Field    string `json:"field"`    // dynamic values from DB
	Operator string `json:"operator"` //
	/* Available operators */
	Value interface{} `json:"value"` // user input , for now it based on field data type
	// the available field data types are string, int, float, boolean, json
}

type TimeCondition struct {
	Operator string `json:"operator"`
	/* Available time operators */
	Value     any    `json:"value,omitempty"`      // if operator is on, before, after the value is date string (2026-02-26) and for last_n_days, next_n_days the value is int
	StartDate string `json:"start_date,omitempty"` // user input string and only appears if operator is between
	EndDate   string `json:"end_date,omitempty"`   // user input string and only appears if operator is between
}

type CountCondition struct {
	Operator string `json:"operator"`
	/* Available count operators */
	Value interface{} `json:"value,omitempty"` // can be int or string , for operator on ,the value is string and for last_n_days, next_n_days the value is int
	Min   int         `json:"min,omitempty"`   // user input is int and only appears if operator is between
	Max   int         `json:"max,omitempty"`   // user input is int and only appears if operator is between
}

type StringOrNumber struct {
	Str *string
	Num *float64
}

// Refer this for available operators:

// [
//   { name: '=', value: '=', label: '=' },
//   { name: '!=', value: '!=', label: '!=' },
//   { name: '<', value: '<', label: '<' },
//   { name: '>', value: '>', label: '>' },
//   { name: '<=', value: '<=', label: '<=' },
//   { name: '>=', value: '>=', label: '>=' },
//   { name: 'contains', value: 'contains', label: 'contains' },
//   { name: 'beginsWith', value: 'beginsWith', label: 'begins with' },
//   { name: 'endsWith', value: 'endsWith', label: 'ends with' },
//   { name: 'doesNotContain', value: 'doesNotContain', label: 'does not contain' },
//   { name: 'doesNotBeginWith', value: 'doesNotBeginWith', label: 'does not begin with' },
//   { name: 'doesNotEndWith', value: 'doesNotEndWith', label: 'does not end with' },
//   { name: 'null', value: 'null', label: 'is null' },
//   { name: 'notNull', value: 'notNull', label: 'is not null' },
//   { name: 'in', value: 'in', label: 'in' },
//   { name: 'notIn', value: 'notIn', label: 'not in' },
//   { name: 'between', value: 'between', label: 'between' },
//   { name: 'notBetween', value: 'notBetween', label: 'not between' },
// ];

// Available time operators
// last_n_days
// next_n_days
// on
// before
// after
// between

// Availble count operators
// greater_than
// greater_than_or_equal
// less_than
// less_than_or_equal
// between
// equal
