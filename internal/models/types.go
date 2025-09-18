// internal/rqbmodel/types.go
package models

type RqbRoot struct {
	ID         string    `json:"id"`
	Rules      []RqbRule `json:"rules"`
	Combinator string    `json:"combinator"`
	Not        bool      `json:"not"`
}

type RqbRule struct {
	ID          string      `json:"id"`
	Field       string      `json:"field"`
	Operator    string      `json:"operator"`
	ValueSource string      `json:"valueSource"`
	Value       interface{} `json:"value"`
	Combinator  string      `json:"combinator,omitempty"`
	Not         bool        `json:"not,omitempty"`
	Match       *struct {
		Mode      string `json:"mode"`
		Threshold int    `json:"threshold"`
	} `json:"match,omitempty"`
}
