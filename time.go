package main

import (
	//"bytes"
	"errors"
	"fmt"
	//"io/ioutil"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	DaysRe  = regexp.MustCompile(`^(\d+)d`)
	HoursRe = regexp.MustCompile(`(\d+)h`)
	DateRe  = regexp.MustCompile(`^(\w{3})\s(\d{2})\s(\d{2}:\d{2}:\d{2})`)
)

var (
	Months = []string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}
)

var (
	ErrInvalidMonth    = errors.New("Invalid month")
	ErrInvalidDuration = errors.New("Invalid duration")
	ErrInvalidMdhms    = errors.New("Invalid MDhms")
)

// Parse a `time.Month` from a 3 character month string
func parseMonth(month string) (m time.Month, e error) {
	month = strings.ToLower(month)
	index := -1
	m = time.Month(0)
	e = nil
	for i, mon := range Months {
		if mon == month {
			index = i + 1
		}
	}
	if index < 0 {
		e = ErrInvalidMonth
		return
	}
	m = time.Month(index)
	return
}

// Convert a "{numberOfDays}d" to a "{numberOfHours}h" format to be parsable by `time.ParseDuration`
// if "{numberOfHours}h" exists it is replaced summed with `numberOfhours + (numberOfDays * 24)`
func normalizeDuration(s string) (str string, e error) {
	str = s
	if DaysRe.MatchString(s) {
		dm := DaysRe.FindAllStringSubmatch(s, 1)
		n, err := strconv.ParseInt(dm[0][1], 10, 32)
		if err != nil {
			e = ErrInvalidDuration
			return
		}
		deltaH := int32(n) * 24
		if HoursRe.MatchString(s) {
			hm := HoursRe.FindAllStringSubmatch(s, 1)
			n, err = strconv.ParseInt(hm[0][1], 10, 32)
			if err != nil {
				e = ErrInvalidDuration
				return
			}
			deltaH += int32(n)
		}
		str = DaysRe.ReplaceAllString(str, "")
		str = HoursRe.ReplaceAllString(str, "")
		str = fmt.Sprintf("%dh%s", deltaH, str)
	}
	return
}

// Short hand for `time.ParseDuration(normalizeDuration({durationString}))`
func parseDuration(s string) (d time.Duration, e error) {
	d = time.Duration(0)
	s, e = normalizeDuration(s)
	if e != nil {
		return
	}
	d, e = time.ParseDuration(s)
	return
}

func parseTimeMdhms(m, d, hms string) (t time.Time, e error) {
	t = time.Time{}
	hmsA := strings.Split(hms, ":")
	month, mErr := parseMonth(m)
	day, dErr := strconv.ParseInt(d, 10, 8)
	hour, hErr := strconv.ParseInt(hmsA[0], 10, 8)
	min, minErr := strconv.ParseInt(hmsA[1], 10, 8)
	sec, sErr := strconv.ParseInt(hmsA[2], 10, 8)
	if mErr != nil || dErr != nil || hErr != nil || minErr != nil || sErr != nil {
		e = ErrInvalidMdhms
		return
	} else {
		now := time.Now()
		t = time.Date(now.Year(), month, int(day), int(hour), int(min), int(sec), 0, now.Location())
	}
	return
}

/*
	DEPRECATED


func getLineTime(line []byte) (t time.Time) {
	t = time.Time{}
	if !DateRe.Match(line) {
		return
	}
	m := DateRe.FindAllStringSubmatch(string(line), -1)
	t, _ = parseTimeMdhms(m[0][1], m[0][2], m[0][3])
	return
}

func getLineStartIndex(lines [][]byte, d time.Duration) (index int) {
	var lIndex int
	var i int
	reverse := false
	now := time.Now()
	since := now.Add(-1 * d)
	size := len(lines)
	if len(lines[size-1]) == 0 {
		lIndex = size - 2
	} else {
		lIndex = size - 1
	}

	first := getLineTime(lines[0])
	last := getLineTime(lines[lIndex])
	if !first.IsZero() && !first.Before(since) {
		index = 0
		return
	}
	if !last.IsZero() && last.Before(since) {
		index = -1
		return
	}
	// Start from beggining or end
	if !first.IsZero() && !last.IsZero() {
		fSub := first.Sub(since)
		lSub := last.Sub(since)
		if math.Abs(float64(fSub)) < math.Abs(float64(lSub)) {
			i = 0
		} else {
			i = lIndex
			reverse = true
		}
	}

	for {
		line := lines[i]
		t := getLineTime(line)
		if !t.IsZero() {
			if reverse {
				if t.Before(since) {
					break
				}
			} else {
				if t.After(since) {
					break
				}
			}
		}

		if reverse {
			if i == 0 {
				break
			}
			i--
		} else {
			if i == size-1 {
				break
			}
			i++
		}
	}
	index = i
	return
}
*/
