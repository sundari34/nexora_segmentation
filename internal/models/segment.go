package models

import "github.com/nexora/nexora_segmentation/internal/utils"

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
	EventName string    `json:"event_name"` // e.g., "App opened"
	EventID   int       `json:"event_id"`   // UI event id (optional mapping)
	Condition string    `json:"condition"`  // e.g., "has_performed"
	Time      TimeRule  `json:"time"`
	Count     CountRule `json:"count"`
	Query     *string   `json:"query"` // RQB JSON string or null
}

// type TimeRule struct {
// 	Operator string  `json:"operator"`
// 	Start    *string `json:"start,omitempty"`
// 	End      *string `json:"end,omitempty"`
// 	Value    *string `json:"value,omitempty"`
// 	DayValue *int    `json:"day_value,omitempty"`
// }

// type CountRule struct {
// 	Operator string `json:"operator"` // equal_to, greater_than, less_than, at_least, at_most
// 	Value    string `json:"value"`    // integer as string
// }

type TimeRule struct {
	Operator string                `json:"operator"`
	Start    *utils.FlexibleString `json:"start,omitempty"`
	End      *utils.FlexibleString `json:"end,omitempty"`
	Value    *utils.FlexibleString `json:"value,omitempty"`
	DayValue *int                  `json:"day_value,omitempty"`
}

type CountRule struct {
	Operator string               `json:"operator"`
	Value    utils.FlexibleString `json:"value"`
}

// Response member
type Member struct {
	NexoraID string `json:"nexora_id"`
	ClientID string `json:"client_id"`
}
