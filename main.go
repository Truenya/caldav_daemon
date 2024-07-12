package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/martinlindhe/notify"
)

const (
	defaultTimeBeforeNotify = 5 * time.Minute
	defaultRefreshPeriod    = 10 * time.Minute
	timeFormat              = "1/02/2006 15:04"
	tzFile                  = "/etc/timezone"
	messageAboutOffsets     = "if event is in future, probably your timezone is not same as on your server, than please set offset in the CALDAV_SERVER_OFFSET_HOURS env var\n"
	notifyEnvIcon           = "CALDAV_NOTIFY_ICON"
	notifyEnvOffset         = "CALDAV_SERVER_OFFSET_HOURS"
	notifyEnvRefreshPeriod  = "CALDAV_REFRESH_PERIOD_MINUTES"
	notifyEnvTimeBefore     = "CALDAV_NOTIFY_BEFORE_MINUTES"
	notifyEnvWithSound      = "CALDAV_NOTIFY_WITH_SOUND"
)

func main() {
	// TODO delete on first external contribution
	notify.Notify("", "https://github.com/Truenya", "notification daemon started", "")
	events := make(chan event)
	go planEvents(events)

	icon := os.Getenv(notifyEnvIcon)
	for e := range events {
		if sound := os.Getenv(notifyEnvWithSound); sound != "" {
			notify.Alert("", e.Summary, e.Description, icon)
		} else {
			notify.Notify("", e.Summary, e.Description, icon)
		}
	}
}

func planEvents(ch chan event) {
	planned := map[string]struct{}{}
	offset := getOffsetByServer()
	period := getRefreshPeriod()
	timeBefore := getTimeBeforeNotify()
	for {
		events := getEvents(offset)
		checkAndRememberEvents(ch, events, planned, timeBefore)
		time.Sleep(period)
	}
}

func checkAndRememberEvents(ch chan event, events []event, planned map[string]struct{}, timeBefore time.Duration) {
	for _, e := range events {
		key := e.Summary + e.Start.String()
		if _, ok := planned[key]; ok || e.isPast() {
			if e.isToday() {
				fmt.Printf("skipping event from past: %s at: %s\n %s", e.Summary, e.Start.String(), messageAboutOffsets)
			}
			continue
		}

		go e.notify(ch, timeBefore)
		planned[key] = struct{}{}

		fmt.Println("planned event:", e.Summary, "at:", e.Start.String())
	}
}

func (e event) notify(ch chan event, timeBefore time.Duration) {
	dur := time.Until(e.Start) - timeBefore
	if dur > 0 {
		fmt.Println("sleeping for:", dur, "for event:", e.Summary)
		time.Sleep(dur)
	}
	ch <- e
	fmt.Printf("notified for event: %+v", e)
}

func getEvents(offset time.Duration) []event {
	out, err := exec.Command("caldav-fetch.py").Output()
	if err != nil {
		processExitError(err)
	}

	data := []eventRaw{}
	if err := json.Unmarshal(out, &data); err != nil {
		panic(err)
	}

	result := make([]event, len(data))
	for i, e := range data {
		e, err := eventFromEventRaw(e, offset)
		if err != nil {
			fmt.Printf("failed to parse event: %+v", e)
			continue
		}
		result[i] = e
	}

	return result
}

type eventRaw struct {
	Name        string
	Start       string
	End         string
	Datestamp   string
	Summary     string
	Description string
}

type event struct {
	Name        string
	Summary     string
	Description string
	Start       time.Time
	End         time.Time
	CreatedAt   time.Time
}

func (e event) isPast() bool {
	return e.Start.Before(time.Now()) && e.End.Before(time.Now())
}

func (e event) isToday() bool {
	return e.Start.Day() == time.Now().Day()
}

func eventFromEventRaw(raw eventRaw, offset time.Duration) (event, error) {
	start, err := time.Parse(timeFormat, raw.Start)
	if err != nil {
		return event{}, err
	}
	end, _ := time.Parse(timeFormat, raw.End)
	datestamp, _ := time.Parse(timeFormat, raw.Datestamp)

	if offset == 0 {
		// The time comes according to the server time zone, but is read as UTC, so we subtract offset
		// Let's assume that the server is in the same zone as the client
		// Otherwise, offset must be set in the corresponding environment variable
		_, offset := time.Now().Zone()

		// change time by timezone of current location
		start = start.Add(-time.Duration(offset) * time.Second)
		end = end.Add(-time.Duration(offset) * time.Second)
		datestamp = datestamp.Add(-time.Duration(offset) * time.Second)
	}

	return event{
		Name:        raw.Name,
		Summary:     raw.Summary,
		Description: raw.Description,
		Start:       start,
		End:         end,
		CreatedAt:   datestamp,
	}, nil
}

func getRefreshPeriod() time.Duration {
	period := os.Getenv(notifyEnvRefreshPeriod)
	if period == "" {
		return defaultRefreshPeriod
	}
	d, err := strconv.Atoi(period)
	if err != nil {
		fmt.Println("error parsing refresh period:", err)
		return defaultRefreshPeriod
	}
	return time.Duration(d) * time.Minute
}

func getTimeBeforeNotify() time.Duration {
	period := os.Getenv(notifyEnvTimeBefore)
	if period == "" {
		return defaultTimeBeforeNotify
	}
	d, err := strconv.Atoi(period)
	if err != nil {
		fmt.Println("error parsing time before notify:", err)
		return defaultTimeBeforeNotify
	}
	return time.Duration(d) * time.Minute
}

func getOffsetByServer() time.Duration {
	offsetHours := os.Getenv(notifyEnvOffset)
	if offsetHours == "" {
		return 0
	}

	intOffset, err := strconv.Atoi(offsetHours)
	if err != nil {
		return 0
	}

	return time.Duration(intOffset) * time.Hour
}

func processExitError(err error) {
	ee := &exec.ExitError{}
	if !errors.As(err, &ee) || !strings.Contains(string(ee.Stderr), "Unauthorized") {
		fmt.Printf("failed to run caldav-fetch.py: \n%s", err)
		os.Exit(1)
	}

	fmt.Println("Unauthorized. Please check your credentials in python script.")
	os.Exit(1)
}
