#!/usr/bin/env python3
import os
import sys
import csv
import requests

BASE_URL = "https://bug-bash.innovations-sandbox.sonatype.dev"
#BASE_URL = "http://localhost:7777"

# CAMPAIGN_NAME = "cncf"
CAMPAIGN_NAME = "cncf-2021-10-11"

# CSV_COLUMN_NAME_GITHUB_ID = "GitHub ID"
CSV_COLUMN_NAME_GITHUB_ID = "Git Handle"
CSV_COLUMN_NAME_DISPLAY_NAME = "Full Name"

if len(sys.argv) != 2:
    print("Usage: ./participants.py CSV")
    exit(1)

with open(sys.argv[1], newline='') as csvfile:
    print(f"adding participants from: {csvfile.name}")
    participants = csv.DictReader(csvfile)
    rows = list(participants)
    count_added = 0
    count_skipped = 0
    count_error = 0
    exitCode = 0
    for participant in rows:
        print(f"participant: {participant}")

        # detect email instead of required github ID
        if "@" in participant[CSV_COLUMN_NAME_GITHUB_ID]:
            print(f"*** invalid Github ID detected: participant: {participant}.*** \n*** Skipping participant.  ***")
            count_error += 1
            exitCode = 1
            continue

        # use suffix of github url if github ID is a urllib3
        if "/" in participant[CSV_COLUMN_NAME_GITHUB_ID]:
            participant[CSV_COLUMN_NAME_GITHUB_ID] = os.path.basename(participant[CSV_COLUMN_NAME_GITHUB_ID])

        # make sure github id is lower case
        participant[CSV_COLUMN_NAME_GITHUB_ID] = participant[CSV_COLUMN_NAME_GITHUB_ID].lower()

        send = {
            'campaignName': CAMPAIGN_NAME,
            'GithubName': participant[CSV_COLUMN_NAME_GITHUB_ID],
            # 'email': participant['Email'],
            'DisplayName': participant[CSV_COLUMN_NAME_DISPLAY_NAME]
        }
        r = requests.request('GET', f"{BASE_URL}/participant/detail/{participant[CSV_COLUMN_NAME_GITHUB_ID]}")
        if r.status_code == 200:
            count_skipped += 1
            print("Participant already exists, skipping")
        else:
            print(f"Adding participant: {participant[CSV_COLUMN_NAME_GITHUB_ID]}")
            r = requests.request('PUT', f"{BASE_URL}/participant/add", json=send)
            print(r)
            print(r.text)
            if r.status_code != 200 and r.status_code != 201:
                count_error += 1
            else:
                count_added += 1

    print(f"done processing {len(rows)} participants")
    print(f"added {count_added}")
    print(f"skipped {count_skipped}")
    print(f"errored {count_error}")
    exit(exitCode)
