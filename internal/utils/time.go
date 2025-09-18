package utils

import (
	"fmt"
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

	switch operator {
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
