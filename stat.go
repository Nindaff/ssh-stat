package main

import (
	"bytes"
	//"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/codegangsta/cli"
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

func padTo(v string, to int) string {
	size := len(v)
	diff := to - size
	if diff < 0 {
		return v
	}
	var sp int
	var ep int
	if diff%2 == 0 {
		sp = diff / 2
		ep = sp
	} else {
		n := float64(diff) / 2
		sp = int(math.Floor(n))
		ep = int(math.Ceil(n))
	}
	return fmt.Sprintf("%s%s%s", strings.Repeat(" ", sp), v, strings.Repeat(" ", ep))
}

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

func (e *Entry) Row(color bool) string {
	// |date|ip|attempts|user|server|authenticated|invalidUser|protocol|port|
	s := "|%s|%s|%s|%s|%s|%s|%s|%s|%s|"
	y, m, d := e.Time.Date()
	date := padTo(fmt.Sprintf("%d/%d/%d", d, m, y), 12)
	ip := padTo(e.Ip, 17)
	attempts := padTo(fmt.Sprintf("%d", e.Attempts), 8)
	user := padTo(e.User, 10)
	authenticated := padTo(fmt.Sprintf("%t", e.Authenticated), 6)
	invalidUser := padTo(fmt.Sprintf("%t", e.InvalidUser), 6)
	protocol := padTo(e.Protocol, 6)
	port := padTo(e.Port, 6)
	return fmt.Sprintf(s, date, ip, attempts, user, authenticated, invalidUser,
		protocol, port)
}

func parseEntry(line []byte) (entry *Entry, e error) {
	var matches [][]string
	var auth bool

	if FailedRe.Match(line) {
		matches = FailedRe.FindAllStringSubmatch(string(line), -1)
		auth = false
	} else if AcceptedRe.Match(line) {
		matches = AcceptedRe.FindAllStringSubmatch(string(line), -1)
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
	Failed           []*Entry
	Accepted         []*Entry

	entries map[string]*Entry
	Ips     []string
}

func newResults() *Results {
	return &Results{
		Failed:   []*Entry{},
		Accepted: []*Entry{},
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

func (r *Results) rows(entries []*Entry) string {
	a := make([]string, len(entries))
	for i := 0; i < len(entries); i++ {
		a[i] = entries[i].Row(false)
	}
	return strings.Join(a, "\n")
}

func (r *Results) String() string {
	sort.Sort(Entries(r.Accepted))
	sort.Sort(Entries(r.Failed))
	accepted := r.rows(r.Accepted)
	failed := r.rows(r.Failed)
	r.generateUniqueIps()
	s := `
total attempts: %d
accepted attempts: %d
failed attempts: %d
accepted:
%s
failed:
%s
ips:
%s`

	return fmt.Sprintf(s, r.Attempts, r.AcceptedAttempts,
		r.FailedAttempts, accepted, failed, strings.Join(r.Ips, "\n"))
}

func (r *Results) ColorString() string {
	return "colored results"
}

type Parser struct {
	concurrency int
	sem         chan struct{}
	input       [][]byte
	results     *Results

	closed bool
}

func NewParser(concurrency int, input [][]byte) *Parser {
	return &Parser{
		concurrency: concurrency,
		sem:         make(chan struct{}, concurrency),
		input:       input,
		closed:      false,
		results:     newResults(),
	}
}

func (p *Parser) parseLine(line []byte) {
	entry, err := parseEntry(line)
	if err != nil {
		return
	}
	p.results.AddEntry(entry)
}

func (p *Parser) run() error {
	if p.closed {
		return ErrParserClosed
	}
	wg := sync.WaitGroup{}
	for i := 0; i < len(p.input); i++ {
		p.sem <- struct{}{}
		go func(line []byte) {
			wg.Add(1)
			p.parseLine(line)
			wg.Done()
			<-p.sem
		}(p.input[i])
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

	data, err := ioutil.ReadFile(inputFile)
	if err != nil {
		panic(err)
	}
	lines := bytes.Split(data, []byte("\n"))
	sIndex := 0
	if afterDuration == "" {
		afterDuration = fmt.Sprintf("%dd%dh%dm%ds", days, hours, mins, secs)
	}
	if afterDuration != "" {
		d, err := parseDuration(afterDuration)
		if err != nil && int(d) != 0 {
			sIndex = getLineStartIndex(lines, d)
		}
	}
	lines = lines[sIndex:]
	if cpus < 1 {
		cpus = runtime.NumCPU()
	}
	runtime.GOMAXPROCS(cpus)
	concurrency := 5 * cpus

	parser := NewParser(concurrency, lines)
	err = parser.run()
	if err != nil {
		panic(err)
	}
	results := parser.results
	fmt.Printf("%s\n", results)
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
