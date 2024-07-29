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
	needSound := os.Getenv(notifyEnvWithSound) != ""
	for e := range events {
		if needSound {
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
		checkAndRememberEvents(ch, getEvents(offset), planned, timeBefore)
		time.Sleep(period)
	}
}

func checkAndRememberEvents(ch chan event, events []event, planned map[string]struct{}, timeBefore time.Duration) {
	for _, e := range events {
		key := e.Summary + e.Start.String()
		if _, ok := planned[key]; ok || e.isPast() {
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
		result[i] = repeatedEventFromEvent(e)
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
	Duration    string
	Rrule       *rruleRaw
}

type event struct {
	Name        string
	Summary     string
	Description string
	Start       time.Time
	End         time.Time
	CreatedAt   time.Time
	Rrule       *rrule
}

type rruleRaw struct {
	Freq     []string
	Until    *string
	ByDay    []string
	Interval []int
}

type rrule struct {
	Freq     string
	Until    *time.Time
	ByDay    map[time.Weekday]struct{}
	Interval int
	Duration time.Duration
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
		_, offsetI := time.Now().Zone()
		offset = time.Duration(offsetI) * time.Second
	}

	// change time by timezone of current location
	start = start.Add(-offset)
	end = end.Add(-offset)
	datestamp = datestamp.Add(-offset)

	return event{
		Name:        raw.Name,
		Summary:     raw.Summary,
		Description: raw.Description,
		Start:       start,
		End:         end,
		CreatedAt:   datestamp,
		Rrule:       readRrule(raw.Rrule, offset, raw.Duration),
	}, nil
}

func repeatedEventFromEvent(e event) event {
	if e.Rrule == nil || (e.Rrule.Until != nil && e.Rrule.Until.Before(time.Now())) {
		return e
	}

	if !e.Rrule.checkByFreq(e.Start) {
		return e
	}

	today0 := time.Now().UTC().Truncate(24 * time.Hour)
	e.Start = today0.Add(offsetFromTime(e.Start))
	e.End = e.Start.Add(e.Rrule.Duration)

	return e
}

func (r rrule) checkByFreq(start time.Time) bool {
	switch r.Freq {
	case "WEEKLY":
		if r.ByDay == nil {
			return false
		}

		if _, ok := r.ByDay[time.Now().Weekday()]; !ok {
			return false
		}

		_, w := time.Now().ISOWeek()
		_, w2 := start.ISOWeek()

		return (w2-w)%r.Interval == 0
	case "DAILY":
		return (start.Day()-time.Now().Day())%r.Interval == 0
	case "MONTHLY":
		return (int(start.Month())-int(time.Now().Month()))%r.Interval == 0
	case "YEARLY":
		return (start.Year()-time.Now().Year())%r.Interval == 0
	default:
		return false
	}
}

func offsetFromTime(t time.Time) time.Duration {
	h, m, s := t.Clock()
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second
}

func readRrule(raw *rruleRaw, offset time.Duration, dur string) *rrule {
	if raw == nil {
		return nil
	}

	res := &rrule{
		Freq:     getFirst(raw.Freq),
		Interval: getFirst(raw.Interval),
		Until:    parseTime(raw.Until, offset),
		Duration: durationFromString(dur),
	}

	if len(raw.ByDay) == 0 {
		return res
	}

	res.ByDay = make(map[time.Weekday]struct{})
	for _, day := range raw.ByDay {
		res.ByDay[dayFromString(day)] = struct{}{}
	}

	return res
}

func getFirst[T any](arr []T) T {
	if len(arr) == 0 {
		return *new(T)
	}
	return arr[0]
}

func parseTime(raw *string, offset time.Duration) *time.Time {
	if raw == nil {
		return nil
	}
	res, _ := time.Parse(timeFormat, *raw)
	res = res.Add(-offset)
	return &res
}

func durationFromString(dur string) time.Duration {
	tmp := strings.Split(dur, ":")
	if len(tmp) < 3 {
		return 0
	}
	dur = tmp[0] + "h" + tmp[1] + "m" + tmp[2] + "s"
	res, err := time.ParseDuration(dur)
	if err != nil {
		return 0
	}
	return res
}

func dayFromString(day string) time.Weekday {
	switch day {
	case "MO":
		return time.Monday
	case "TU":
		return time.Tuesday
	case "WE":
		return time.Wednesday
	case "TH":
		return time.Thursday
	case "FR":
		return time.Friday
	case "SA":
		return time.Saturday
	case "SU":
		return time.Sunday
	default:
		return time.Sunday
	}
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

func (e event) isPast() bool {
	return e.Start.Before(time.Now()) && e.End.Before(time.Now())
}
