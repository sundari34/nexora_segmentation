package utils

import (
	"strconv"
)

// FlexibleString can hold either a string or a number (int/float),
// and provides helpers to safely convert.
type FlexibleString struct {
	Value string
}

// UnmarshalJSON allows FlexibleString to accept either a number or a string in JSON.
func (fs *FlexibleString) UnmarshalJSON(data []byte) error {
	// If quoted string
	if len(data) > 0 && data[0] == '"' {
		fs.Value = string(data[1 : len(data)-1])
		return nil
	}
	// Otherwise assume number
	fs.Value = string(data)
	return nil
}

// ToInt converts the underlying value to int.
// func (fs *FlexibleString) ToInt() (int, error) {
// 	if fs == nil || fs.Value == "" {
// 		return 0, fmt.Errorf("empty FlexibleString")
// 	}
// 	return strconv.Atoi(fs.Value)
// }

func (fs *FlexibleString) ToInt() (int, error) {
	if fs == nil || fs.IsEmpty() {
		return 0, nil // return 0 for empty instead of error
	}
	return strconv.Atoi(fs.Value)
}

// String returns the raw string value.
func (fs *FlexibleString) String() string {
	if fs == nil {
		return ""
	}
	return fs.Value
}

// IsEmpty checks if FlexibleString is nil or empty.
func (fs *FlexibleString) IsEmpty() bool {
	return fs == nil || fs.Value == ""
}
