package models

// Payload matches what your UI sends
type SegmentPayload struct {
	SegmentID      string      `json:"segment_id,omitempty"` // optional: pass via payload or query param
	SegmentName    string      `json:"segment_name"`
	SegmentType    string      `json:"segment_type"`    // e.g., "past"
	GroupCondition string      `json:"group_condition"` // "and" / "or" (across groups)
	Groups         []RuleGroup `json:"groups"`
}

type RuleGroup struct {
	MatchMode string   `json:"matchMode"` // "and" / "or" within a group
	Filters   []Filter `json:"filters"`
}

type Filter struct {
	EventType string    `json:"event_type"` // e.g., "System Events"
	EventID   int       `json:"event_id"`   // UI event id (optional mapping)
	Condition string    `json:"condition"`  // e.g., "has_performed"
	Time      TimeRule  `json:"time"`
	Count     CountRule `json:"count"`
	Query     *string   `json:"query"` // RQB JSON string or null
}

type TimeRule struct {
	Operator string  `json:"operator"` // e.g., "last_n_days", "before", "after", "between"
	Start    *string `json:"start"`    // ISO "YYYY-MM-DD" when relevant
	End      *string `json:"end"`      // ISO "YYYY-MM-DD" when relevant
	Value    *string `json:"value"`    // e.g., "7" for last_n_days
}

type CountRule struct {
	Operator string `json:"operator"` // equal_to, greater_than, less_than, at_least, at_most
	Value    string `json:"value"`    // integer as string
}

// Response member
type Member struct {
	NexoraID string `json:"nexora_id"`
	ClientID string `json:"client_id"`
}
