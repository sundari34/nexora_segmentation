package utils

import (
	"fmt"
	"time"
)

// Returns [startDate, endDate] inclusive, in "2006-01-02" format.
// Supports: last_n_days, before, after, between (start/end provided)
func DeriveDateRange(op string, start, end, value *string, now time.Time, loc *time.Location) (string, string, error) {
	today := now.In(loc)
	switch op {
	case "last_n_days":
		// include today; last N days means [today-(N-1), today]
		if value == nil {
			return "", "", fmt.Errorf("time.value required for last_n_days")
		}
		n, err := parseInt(*value)
		if err != nil || n <= 0 {
			return "", "", fmt.Errorf("invalid last_n_days value")
		}
		s := today.AddDate(0, 0, -(n - 1))
		return s.Format("2006-01-02"), today.Format("2006-01-02"), nil
	case "before":
		if start == nil {
			return "", "", fmt.Errorf("time.start required for 'before'")
		}
		// before X => [-inf, start-1]
		st, err := time.ParseInLocation("2006-01-02", *start, loc)
		if err != nil {
			return "", "", err
		}
		e := st.AddDate(0, 0, -1)
		return "1970-01-01", e.Format("2006-01-02"), nil
	case "after":
		if start == nil {
			return "", "", fmt.Errorf("time.start required for 'after'")
		}
		// after X => [start+1, today]
		st, err := time.ParseInLocation("2006-01-02", *start, loc)
		if err != nil {
			return "", "", err
		}
		s := st.AddDate(0, 0, 1)
		return s.Format("2006-01-02"), today.Format("2006-01-02"), nil
	case "between":
		if start == nil || end == nil {
			return "", "", fmt.Errorf("time.start and time.end required for 'between'")
		}
		return *start, *end, nil
	default:
		return "", "", fmt.Errorf("unsupported time.operator: %s", op)
	}
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
