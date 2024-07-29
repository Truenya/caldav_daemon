#!/usr/bin/env python3
import json
from os import environ as env

import caldav

username = env["CALDAV_USERNAME"]
password = env["CALDAV_PASSWORD"]
url = env["CALDAV_URL"]
caldav_url = f"https://{url}/{username}/"
headers = {}


def fetch_and_print():
    with caldav.DAVClient(
        url=caldav_url,
        username=username,
        password=password,
        # Optional parameter to set HTTP headers on each request if needed
        headers=headers,
    ) as client:
        print_calendars_demo(client.principal().calendars())


def print_calendars_demo(calendars):
    if not calendars:
        return
    events = []
    for calendar in calendars:
        for event in calendar.events():
            for component in event.icalendar_instance.walk():
                if component.name != "VEVENT" or component.get('STATUS') == 'CANCELLED':
                    continue
                events.append(fill_event(component, calendar))
    print(json.dumps(events, indent=2, ensure_ascii=False))


def fill_event(component, calendar) -> dict[str, str]:
    cur = {}
    cur["calendar"] = f"{calendar}"
    cur["summary"] = component.get("summary")
    cur["description"] = component.get("description")
    cur["start"] = component.get("dtstart").dt.strftime("%m/%d/%Y %H:%M")
    endDate = component.get("dtend")
    if endDate and endDate.dt:
        cur["end"] = endDate.dt.strftime("%m/%d/%Y %H:%M")
    cur["datestamp"] = component.get("dtstamp").dt.strftime("%m/%d/%Y %H:%M")
    duration = component.get("duration")
    if duration:
        cur["duration"] = str(duration.dt)
    rrule = component.get('rrule')
    if rrule:
        cur["rrule"] = fill_rrule(rrule)
    return cur


def fill_rrule(rrule) -> dict[str, str]:
    res = dict()
    res['freq'] = rrule.get('freq')
    res['interval'] = rrule.get('interval')

    until = rrule.get("until")
    if until is not None:
        res['until'] = until[0].strftime("%m/%d/%Y %H:%M")

    days = rrule.get('byday')
    if days:
        res['byday'] = []
        for d in days:
            res['byday'].append(d)
    return res


if __name__ == "__main__":
    fetch_and_print()
