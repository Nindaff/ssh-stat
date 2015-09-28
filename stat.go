package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/codegangsta/cli"
)

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

func (p *Parser) Run() error {
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
	orderExp := c.String("order")
	//colors := c.Bool("colors")
	ipOutput := c.Bool("ips")
	acceptedOutput := c.Bool("accepted")
	failedOutput := c.Bool("failed")
	jsonOutput := c.Bool("json")
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

	order := parseOrderBy(orderExp)
	if order == Order_Error {
		fmt.Println("Order expession could not be interpreted")
		os.Exit(1)
	}

	results := newResults(order)
	if acceptedOutput && !failedOutput {
		results.failedOutput = false
	}
	if failedOutput && !acceptedOutput {
		results.acceptedOutput = false
	}
	if ipOutput {
		results.ipOutput = true
	}
	if jsonOutput {
		results.jsonOutput = true
	}

	parser := NewParser(concurrency, f, after, results)
	err = parser.Run()
	if err != nil {
		panic(err)
	}
	results.Render()
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
			Usage: "Only display ip addresses",
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
		cli.StringFlag{
			Name:  "order",
			Value: "attemptsDesc",
			Usage: "Order results, (attemptsAsc, attemptsDesc, chronoAsc, chronoDesc)",
		},
	}
	app.Action = sshStat
	app.Run(os.Args)
}
