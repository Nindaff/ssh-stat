package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/olekukonko/tablewriter"
)

type Results struct {
	sync.Mutex

	Attempts         int
	FailedAttempts   int
	AcceptedAttempts int
	Failed           *EntrySet
	Accepted         *EntrySet
	Ips              []string

	entries        map[string]*Entry
	order          OrderBy
	jsonOutput     bool
	ipOutput       bool
	failedOutput   bool
	acceptedOutput bool
}

func newResults(order OrderBy) *Results {
	return &Results{
		Failed:         newEntrySet(order),
		Accepted:       newEntrySet(order),
		entries:        map[string]*Entry{},
		Ips:            []string{},
		order:          order,
		acceptedOutput: true,
		failedOutput:   true,
		ipOutput:       false,
		jsonOutput:     false,
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
		r.Accepted.Append(entry)
	} else {
		r.Failed.Append(entry)
	}
}

func (r *Results) writeRows(t *tablewriter.Table, es *EntrySet) {
	for _, e := range es.Entries {
		t.Append(e.Row())
	}
}

func (r *Results) renderJSON() {
	fmt.Println("JSON")
}

func (r *Results) renderIps() {
	if r.acceptedOutput && r.failedOutput {
		fmt.Printf("\nAccepted: \n\n%s\n", strings.Join(r.Accepted.Ips(), "\n"))
		fmt.Printf("\nFailed: \n\n%s\n", strings.Join(r.Failed.Ips(), "\n"))
	}
	var ips []string
	if r.acceptedOutput {
		ips = r.Accepted.Ips()
	} else {
		ips = r.Failed.Ips()
	}
	fmt.Println(strings.Join(ips, "\n"))
}

func (r *Results) renderTable() {
	headers := []string{"Date", "Ip", "Attempts", "User", "Auth", "Protocol", "Port", "Server"}
	if r.acceptedOutput {
		acceptedTable := tablewriter.NewWriter(os.Stdout)
		acceptedTable.SetHeader(headers)
		r.writeRows(acceptedTable, r.Accepted)
		fmt.Println("Accepted: \n")
		acceptedTable.Render()
	}

	if r.failedOutput {
		failedTable := tablewriter.NewWriter(os.Stdout)
		failedTable.SetHeader(headers)
		r.writeRows(failedTable, r.Failed)
		fmt.Println("Failed: \n")
		failedTable.Render()
	}
}

func (r *Results) Render() {
	r.Accepted.Sort()
	r.Failed.Sort()
	switch {
	case r.ipOutput:
		r.renderIps()
	case r.jsonOutput:
		r.renderJSON()
	default:
		r.renderTable()
	}
}
