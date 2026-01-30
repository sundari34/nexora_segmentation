package models

import "encoding/json"

type SegmentNewPayload struct {
	GroupCondition string         `json:"group_condition,omitempty"`
	Groups         []RuleGroupRaw `json:"groups,omitempty"`
	Property       string         `json:"property,omitempty"`
	Channel        string         `json:"channel,omitempty"`
	NexoraIDs      string         `json:"nexora_id,omitempty"`
	ClientID       string         `json:"client_id,omitempty"`
	ProjectID      string         `json:"project_id,omitempty"`
	IsNeedCount    bool           `json:"need_count,omitempty"`
	IsNeedSQL      bool           `json:"need_sql,omitempty"`
	IsNeedValue    bool           `json:"need_value,omitempty"`
	Category       string         `json:"category"`
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
	Condition string          `json:"condition"`
}

type UserPropertyCondition struct {
	UserPropertyQuery *QueryBlock `json:"user_property_query"`
}

type QueryBlock struct {
	Combinator string `json:"combinator"`
	Rules      []Rule `json:"rules"`
}

type Rule struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

type TimeCondition struct {
	Operator  string `json:"operator"`
	Value     string `json:"value,omitempty"`
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
}

type CountCondition struct {
	Operator string `json:"operator"`
	Value    int    `json:"value,omitempty"`
	Min      int    `json:"min,omitempty"`
	Max      int    `json:"max,omitempty"`
}
