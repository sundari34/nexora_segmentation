package models

import (
	"encoding/json"
	"fmt"

	"github.com/nexora/nexora_segmentation/internal/utils"
)

// Payload matches what your UI sends (old *or* new)
type SegmentPayload struct {
	SegmentID      string      `json:"segment_id,omitempty"` // optional: pass via payload or query param
	SegmentName    string      `json:"segment_name,omitempty"`
	SegmentType    string      `json:"segment_type,omitempty"`    // e.g., "past"
	GroupCondition string      `json:"group_condition,omitempty"` // "and" / "or" (across groups)
	Groups         []RuleGroup `json:"groups,omitempty"`
	Property       string      `json:"property,omitempty"`
	Channel        string      `json:"channel,omitempty"`
	NexoraID       string      `json:"nexora_id,omitempty"`
	ClientID       string      `json:"client_id,omitempty"`
	ProjectID      string      `json:"project_id,omitempty"`
}

type SegmentNewPayload struct {
	GroupCondition string      `json:"group_condition,omitempty"` // "and" / "or" (across groups)
	Groups         []RuleGroup `json:"groups,omitempty"`
	Property       string      `json:"property,omitempty"`
	Channel        string      `json:"channel,omitempty"`
	NexoraIDs      []string    `json:"nexora_id,omitempty"`
	ClientID       string      `json:"client_id,omitempty"`
	ProjectID      string      `json:"project_id,omitempty"`
	IsNeedCount    bool        `json:"need_count,omitempty"`
	IsNeedSQL      bool        `json:"need_sql,omitempty"`
	IsNeedValue    bool        `json:"need_value,omitempty"`
	Category       string      `json:"category"`
}

// NOTE: new top-level payload is an array of group-like objects.
// To remain backwards compatible we implement a custom Unmarshal below
// which accepts either:
//   - old object: { "groups": [...] , "segment_name": ... }
//   - new array: [ { "filters": [...], "matchMode": "and" }, ... ]
func (sp *SegmentPayload) UnmarshalJSON(b []byte) error {
	// try the object form first
	type alias SegmentPayload
	var obj alias
	if err := json.Unmarshal(b, &obj); err == nil && (len(obj.Groups) > 0 || obj.SegmentName != "") {
		*sp = SegmentPayload(obj)
		return nil
	}

	// fallback: try array-of-groups form
	var groups []RuleGroup
	if err := json.Unmarshal(b, &groups); err == nil && len(groups) > 0 {
		sp.Groups = groups
		return nil
	}

	// last attempt: maybe it's an empty object, just unmarshal into alias (gives zero values)
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	fmt.Println(obj)
	fmt.Println("((((((((((((((((((((((((((((((((((((((((((((((obj))))))))))))))))))))))))))))))))))))))))))))))")
	*sp = SegmentPayload(obj)
	return nil
}

type RuleGroup struct {
	MatchMode string   `json:"matchMode"` // "and" / "or" within a group
	Filters   []Filter `json:"filters"`
}

// Filter supports both the old flattened schema AND the new nested "condition" schema.
// The evaluator will prefer the Condition block when non-nil.
type Filter struct {
	// Old / flattened fields (kept for backward compatibility)
	EventType string          `json:"event_type,omitempty"`      // e.g., "System Events"
	EventName string          `json:"event_name,omitempty"`      // e.g., "App opened"
	EventID   int             `json:"event_id,omitempty"`        // UI event id (optional mapping)
	Condition string          `json:"condition_block,omitempty"` // e.g., "has_performed"
	Time      TimeRule        `json:"time,omitempty"`
	Count     CountRule       `json:"count,omitempty"`
	Query     json.RawMessage `json:"query,omitempty"` // RQB JSON string or null

	// New / nested form
	ConditionBlock     *ConditionRule      `json:"condition,omitempty"`
	UserPropertyFilter *UserPropertyFilter `json:"user_property_query,omitempty"`
	FilterCategory     string              `json:"filter_category,omitempty"`
}

// New condition block (matches new payload)
type ConditionRule struct {
	Time         ConditionTime  `json:"time,omitempty"`
	Count        ConditionCount `json:"count,omitempty"`
	EventID      int            `json:"event_id,omitempty"`
	Condition    string         `json:"condition,omitempty"`
	EventName    string         `json:"event_name,omitempty"`
	EventType    string         `json:"event_type,omitempty"`
	EventDetails struct {
		ID    int    `json:"id,omitempty"`
		Name  string `json:"name,omitempty"`
		Label string `json:"label,omitempty"`
	} `json:"event_details,omitempty"`
	Query             *RqbRoot `json:"query,omitempty"`
	UserPropertyQuery *RqbRoot `json:"user_property_query,omitempty"`
}

type UserPropertyFilter struct {
	ID         string    `json:"id,omitempty"`
	Rules      []RqbRule `json:"rules,omitempty"`
	Combinator string    `json:"combinator,omitempty"`
	Not        bool      `json:"not,omitempty"`
}

// Old TimeRule kept with extra fields to allow translation from new payload.
type TimeRule struct {
	Operator      string                `json:"operator,omitempty"`
	Start         *utils.FlexibleString `json:"start,omitempty"`
	End           *utils.FlexibleString `json:"end,omitempty"`
	Value         *utils.FlexibleString `json:"value,omitempty"`
	DayValue      *utils.FlexibleString `json:"day_value,omitempty"`
	DayCountValue *utils.FlexibleString `json:"day_count_value,omitempty"`

	// Also accept new-name fields (start_date/end_date) for direct unmarshalling
	StartDate *utils.FlexibleString `json:"start_date,omitempty"`
	EndDate   *utils.FlexibleString `json:"end_date,omitempty"`
}

// Old CountRule kept and expanded to include min/max (new payload supports min/max)
type CountRule struct {
	Operator string                `json:"operator,omitempty"`
	Value    utils.FlexibleString  `json:"value,omitempty"`
	Min      *utils.FlexibleString `json:"min,omitempty"`
	Max      *utils.FlexibleString `json:"max,omitempty"`
}

// ConditionTime/Count types that map 1:1 with new payload (kept small)
type ConditionTime struct {
	Value     *utils.FlexibleString `json:"value,omitempty"`
	StartDate *utils.FlexibleString `json:"start_date,omitempty"`
	EndDate   *utils.FlexibleString `json:"end_date,omitempty"`
	Operator  string                `json:"operator,omitempty"`
}

type ConditionCount struct {
	Max      *utils.FlexibleString `json:"max,omitempty"`
	Min      *utils.FlexibleString `json:"min,omitempty"`
	Value    *utils.FlexibleString `json:"value,omitempty"`
	Operator string                `json:"operator,omitempty"`
}

// Response member
type Member struct {
	NexoraID string `json:"nexora_id"`
	ClientID string `json:"client_id"`
	Property string `json:"property"`
}
