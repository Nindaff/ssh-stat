package main

import (
	//"bytes"
	//"encoding/json"
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/codegangsta/cli"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
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

func (e *Entry) row() []string {
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

type Entries []*Entry

func (entries Entries) Len() int {
	return len(entries)
}

func (entries Entries) Swap(i, j int) {
	entries[i], entries[j] = entries[j], entries[i]
}

// Less is more, default sort will be to show entries with
// attempts for most to least
func (entries Entries) Less(i, j int) bool {
	return entries[i].Attempts > entries[j].Attempts
}

type Results struct {
	sync.Mutex

	Attempts         int
	FailedAttempts   int
	AcceptedAttempts int
	Failed           Entries
	Accepted         Entries
	Ips              []string

	entries map[string]*Entry
}

func newResults() *Results {
	return &Results{
		Failed:   Entries{},
		Accepted: Entries{},
		entries:  map[string]*Entry{},
		Ips:      []string{},
	}
}

func (r *Results) AddEntry(entry *Entry) {
	r.Lock()
	defer r.Unlock()

	ip := entry.Ip
	existingEntry, exists := r.entries[ip]
	if exists {
		existingEntry.Attempts++
		if entry.Authenticated == existingEntry.Authenticated {
			return
		} else {
			entry.Attempts = existingEntry.Attempts
		}
	} else {
		r.entries[ip] = entry
	}
	if entry.Authenticated {
		r.Accepted = append(r.Accepted, entry)
	} else {
		r.Failed = append(r.Failed, entry)
	}
}

func (r *Results) generateUniqueIps() {
	r.Lock()
	defer r.Unlock()

	r.Ips = make([]string, len(r.entries))
	i := 0
	for ip := range r.entries {
		r.Ips[i] = ip
		i++
	}
}

func (r *Results) writeEntries(entries Entries) {
	for i := 0; i < len(entries); i++ {
		fmt.Fprintln(r.tw, entries[i].row())
	}
}

func (r *Results) writeRows(t *tablewriter.Table, entries Entries) {
	for _, e := range entries {
		err := t.Append(e.Row())
		if err != nil {
			fmt.Println("Table Error:", err)
		}
	}
}

func (r *Results) render() {
	sort.Sort(r.Accepted)
	sort.Sort(r.Failed)
	headers := []string{"Date", "Ip", "Attempts", "User", "Auth", "Protocol", "Port", "Server"}

	failed := tablewriter.NewWriter(os.Stdout)
	failed.SetHeader(headers)
	r.writeRows(failed, r.Failed)
	failed.Render()

	//r.writeHeader()
	//r.tw.Init(os.Stdout, 0, 0, 0, ' ', 0)
	//r.tw.Init(os.Stdout, 5, 0, 1, ' ', tabwriter.AlignRight)
	//r.writeHeader()
	//fmt.Println("Failed: \n")
	//r.writeEntries(r.Failed)
	//r.tw.Flush()
	/*
		fmt.Println("Accepted: \n")
		r.writeHeader()
		r.writeEntries(r.Accepted)
		r.tw.Flush()
	*/
}

type Parser struct {
	concurrency int
	sem         chan struct{}
	r           io.Reader
	s           *bufio.Scanner
	after       time.Time
	results     *Results

	closed bool
}

func NewParser(concurrency int, r io.Reader, after time.Time, results *Results) *Parser {
	return &Parser{
		concurrency: concurrency,
		sem:         make(chan struct{}, concurrency),
		r:           r,
		s:           bufio.NewScanner(r),
		after:       after,
		closed:      false,
		results:     results,
	}
}

func (p *Parser) parseLine(line string) {
	entry, err := parseEntry(line)
	if err != nil {
		return
	}
	if entry.Time.After(p.after) {
		p.results.AddEntry(entry)
	}
}

func (p *Parser) run() error {
	if p.closed {
		return ErrParserClosed
	}
	wg := sync.WaitGroup{}

	for p.s.Scan() {
		p.sem <- struct{}{}
		go func(line string) {
			wg.Add(1)
			p.parseLine(line)
			wg.Done()
			<-p.sem
		}(p.s.Text())
	}
	close(p.sem)
	wg.Wait()
	p.closed = true
	return nil
}

func sshStat(c *cli.Context) {
	inputFile := c.String("input")
	afterDuration := c.String("after")
	days := c.Int("days")
	hours := c.Int("hours")
	mins := c.Int("mins")
	secs := c.Int("secs")
	//colors := c.Bool("colors")
	//showIps := c.Bool("ips")
	//accepted := c.Bool("accepted")
	//failed := c.Bool("failed")
	cpus := c.Int("cpus")
	//
	f, err := os.Open(inputFile)
	defer f.Close()
	if err != nil {
		panic(err)
	}
	if afterDuration == "" {
		afterDuration = fmt.Sprintf("%dd%dh%dm%ds", days, hours, mins, secs)
	}
	d, err := parseDuration(afterDuration)
	if err != nil {
		panic(err)
	}
	var after time.Time
	if time.Duration(0) == d {
		after = time.Time{}
	} else {
		after = time.Now().Add(-1 * d)
	}
	if cpus < 1 {
		cpus = runtime.NumCPU()
	}
	runtime.GOMAXPROCS(cpus)
	concurrency := 5 * cpus

	results := newResults()

	parser := NewParser(concurrency, f, after, results)
	err = parser.run()
	if err != nil {
		panic(err)
	}
	results.render()
}

/*

	log-file?
	after
	days
	hours
	mins
	secs
	colors
	ips
	failed
	accepted
	concurrency
	cpu

*/

func main() {
	app := cli.NewApp()
	app.Name = "ssh-stat"
	app.Usage = "Get stats on ssh sessions"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "input, i",
			Value: "/var/log/auth.log",
			Usage: "Parse this file.",
		},
		cli.StringFlag{
			Name:  "after, a",
			Value: "",
			Usage: "Start parsing from {n} duration ago, Ex: 4d2h5m3s",
		},
		cli.IntFlag{
			Name:  "days, d",
			Value: 0,
			Usage: "Start parsing at {n} days ago",
		},
		cli.IntFlag{
			Name:  "hours",
			Value: 0,
			Usage: "Start parsing at {n} hours ago",
		},
		cli.IntFlag{
			Name:  "mins, m",
			Value: 0,
			Usage: "Start parsing at {n} minuetes ago",
		},
		cli.IntFlag{
			Name:  "secs, s",
			Value: 0,
			Usage: "Start parsing at {n} seconds ago",
		},
		cli.BoolFlag{
			Name:  "colors",
			Usage: "Display output with colored text",
		},
		cli.BoolFlag{
			Name:  "ips",
			Usage: "Output ip addresses only",
		},
		cli.BoolFlag{
			Name:  "accepted",
			Usage: "Only display accepted login results",
		},
		cli.BoolFlag{
			Name:  "failed",
			Usage: "Only display failed login results",
		},
		cli.IntFlag{
			Name:  "cpu",
			Value: 0,
			Usage: "Set max cpu usage",
		},
	}
	app.Action = sshStat
	app.Run(os.Args)
}
