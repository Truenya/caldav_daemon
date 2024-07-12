#!/usr/bin/env python3
from icalendar import Calendar
import caldav
import json
from os import environ as env

username = env["CALDAV_USERNAME"]
password = env["CALDAV_PASSWORD"]
url      = env["CALDAV_URL"]
caldav_url = f"https://{url}/{username}/"
headers = {}


def fetch_and_print():
    with caldav.DAVClient(
        url=caldav_url,
        username=username,
        password=password,
        headers=headers,  # Optional parameter to set HTTP headers on each request if needed
    ) as client:
        print_calendar_events_json(client.principal().calendars())


def print_calendar_events_json(calendars):
    if not calendars:
        return
    events = []
    for calendar in calendars:
        for eventraw in calendar.events():
            for component in Calendar.from_ical(eventraw._data).walk():
                if component.name != "VEVENT":
                    continue
                cur = {}
                cur['calendar'] = f'{calendar}'
                cur['summary'] = component.get('summary')
                cur['description'] = component.get('description')
                cur['start'] = component.get('dtstart').dt.strftime('%m/%d/%Y %H:%M')
                endDate = component.get('dtend')
                if endDate and endDate.dt:
                    cur['end'] = endDate.dt.strftime('%m/%d/%Y %H:%M')
                cur['datestamp'] = component.get('dtstamp').dt.strftime('%m/%d/%Y %H:%M')
                events.append(cur)
    print(json.dumps(events, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    fetch_and_print()
