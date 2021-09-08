#!/usr/bin/env python3

import sys
import csv
import requests

BASE_URL="https://bug-bash.innovations-sandbox.sonatype.dev"

if len(sys.argv) != 2:
    print("Usage: ./participants.py CSV")
    exit(1)

with open(sys.argv[1], newline='') as csvfile:
    participants = csv.DictReader(csvfile)
    for participant in participants:
        send = {
            'campaignName': 'cncf',
            'GithubName': participant['GitHub ID'],
            'email': participant['Email'],
            'DisplayName': participant['GitHub ID']
        }
        r = requests.request('GET', f"{BASE_URL}/participant/detail/{participant['GitHub ID']}")
        if r.status_code == 200:
            print("Participant already exists, skipping")
        else:
            print(f"Adding participant: {participant['GitHub ID']}")
            r = requests.request('PUT', f"{BASE_URL}/participant/add", json = send)
            print(r)
            print(r.text)
