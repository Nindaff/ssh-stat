package main

import (
	"errors"
	"regexp"
	"sort"
	"time"

	"github.com/fatih/color"
)

var (
	FailedRe = regexp.MustCompile(
		`(\w{3})\s(\d{2})\s(\d{2}:\d{2}:\d{2})\s(\w+)\ssshd\[\d+\]\:\sFailed\s(\w+)\sfor\s([\w\s]+)\sfrom\s([\d\.]+)\sport\s(\d{1,5})\s([\w\d]+)`)
	AcceptedRe = regexp.MustCompile(
		`(\w{3})\s(\d{2})\s(\d{2}:\d{2}:\d{2})\s(\w+)\ssshd\[\d+\]\:\sAccepted\s(\w+)\sfor\s([\w\s]+)\sfrom\s([\d\.]+)\sport\s(\d{1,5})\s([\w\d]+)`)
	InvalidUserRe = regexp.MustCompile(`invalid\suser\s(\w+)`)
)

var (
	ErrParseEntry   = errors.New("Could not parse entry")
	ErrParserClosed = errors.New("Parser is closed")
)

type Entry struct {
	Month         string    `json:"month"`
	Day           string    `json:"day"`
	Hms           string    `json:"hms"`
	Server        string    `json:"server"`
	AuthType      string    `json:"authType"`
	User          string    `json:"user"`
	Ip            string    `json:"ip"`
	Port          string    `json:"port"`
	Protocol      string    `json:"protocol"`
	InvalidUser   bool      `json:"invalidUser"`
	Authenticated bool      `json:"authenticated"`
	Attempts      int       `json:"attempts"`
	Time          time.Time `json:"time"`
}

func (e *Entry) Row() []string {
	y, m, d := e.Time.Date()
	date := color.BlueString("%d/%d/%d", m, d, y)
	ip := color.RedString("%s", e.Ip)
	attempts := color.GreenString("%d", e.Attempts)
	user := color.YellowString("%s", e.User)
	auth := color.WhiteString("%s", e.AuthType)
	proto := color.CyanString("%s", e.Protocol)
	port := e.Port
	server := e.Server
	return []string{date, ip, attempts, user, auth, proto, port, server}
}

func parseEntry(line string) (entry *Entry, e error) {
	var matches [][]string
	var auth bool

	if FailedRe.MatchString(line) {
		matches = FailedRe.FindAllStringSubmatch(line, -1)
		auth = false
	} else if AcceptedRe.MatchString(line) {
		matches = AcceptedRe.FindAllStringSubmatch(line, -1)
		auth = true
	} else {
		e = ErrParseEntry
		return
	}

	m := matches[0]
	if len(m) < 10 {
		e = ErrParseEntry
		return
	}
	/*
	   Month = m[1]
	   Day = m[2]
	   Time = m[3]
	   Server = m[4]
	   AuthType = m[5]
	   User = m[6]
	   Ip = m[7]
	   Port = m[8]
	   Protocol = m[9]
	*/

	entry = &Entry{
		Month:         m[1],
		Day:           m[2],
		Hms:           m[3],
		Server:        m[4],
		AuthType:      m[5],
		User:          m[6],
		Ip:            m[7],
		Port:          m[8],
		Protocol:      m[9],
		Authenticated: auth,
		Attempts:      1,
	}

	if InvalidUserRe.MatchString(entry.User) {
		entry.InvalidUser = true
		um := InvalidUserRe.FindAllStringSubmatch(entry.User, -1)
		if len(um) > 0 && len(um[0]) > 1 {
			entry.User = um[0][1]
		}
	}

	entry.Time, _ = parseTimeMdhms(entry.Month, entry.Day, entry.Hms)
	return
}

type OrderBy int

const (
	Order_Error OrderBy = iota
	Order_AttemptsDesc
	Order_AttemptsAsc
	Order_ChronoDesc
	Order_ChronoAsc
)

func parseOrderBy(exp string) OrderBy {
	switch exp {
	case "attemptsAsc":
		return Order_AttemptsAsc
	case "attemptsDesc":
		return Order_AttemptsDesc
	case "chronoAsc":
		return Order_ChronoAsc
	case "chronoDesc":
		return Order_ChronoDesc
	default:
		return Order_Error
	}
}

type EntrySet struct {
	Entries []*Entry
	order   OrderBy
}

func newEntrySet(order OrderBy) *EntrySet {
	return &EntrySet{
		Entries: []*Entry{},
		order:   order,
	}
}

func (es *EntrySet) Len() int {
	return len(es.Entries)
}

func (es *EntrySet) Swap(i, j int) {
	es.Entries[i], es.Entries[j] = es.Entries[j], es.Entries[i]
}

func (es *EntrySet) Less(i, j int) bool {
	switch es.order {
	case Order_AttemptsAsc:
		return es.Entries[i].Attempts < es.Entries[j].Attempts
	case Order_ChronoDesc:
		return es.Entries[i].Time.After(es.Entries[j].Time)
	case Order_ChronoAsc:
		return es.Entries[i].Time.Before(es.Entries[j].Time)
	default:
		return es.Entries[i].Attempts > es.Entries[j].Attempts
	}
}

func (es *EntrySet) Append(e *Entry) {
	es.Entries = append(es.Entries, e)
}

func (es *EntrySet) Sort() {
	sort.Sort(es)
}

func (es *EntrySet) Ips() []string {
	ips := make([]string, len(es.Entries))
	for i, e := range es.Entries {
		ips[i] = e.Ip
	}
	return ips
}
