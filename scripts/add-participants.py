#!/usr/bin/env python3

import sys
import csv
import requests

BASE_URL = "https://bug-bash.innovations-sandbox.sonatype.dev"
# BASE_URL = "http://localhost:7777"

CAMPAIGN_NAME = "cncf"

if len(sys.argv) != 2:
    print("Usage: ./participants.py CSV")
    exit(1)

with open(sys.argv[1], newline='') as csvfile:
    print(f"adding participants from: {csvfile.name}")
    participants = csv.DictReader(csvfile)
    rows = list(participants)
    count_added = 0
    count_skipped = 0
    for participant in rows:
        print(f"participant: {participant}")
        send = {
            'campaignName': CAMPAIGN_NAME,
            'GithubName': participant['GitHub ID'],
            'email': participant['Email'],
            'DisplayName': participant['GitHub ID']
        }
        r = requests.request('GET', f"{BASE_URL}/participant/detail/{participant['GitHub ID']}")
        if r.status_code == 200:
            count_skipped += 1
            print("Participant already exists, skipping")
        else:
            count_added += 1
            print(f"Adding participant: {participant['GitHub ID']}")
            r = requests.request('PUT', f"{BASE_URL}/participant/add", json=send)
            print(r)
            print(r.text)
    print(f"done processing {len(rows)} participants")
    print(f"added {count_added}")
    print(f"skipped {count_skipped}")
