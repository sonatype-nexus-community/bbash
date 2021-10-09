#!/usr/bin/env python3
import os
import sys
import csv
import requests

BASE_URL = "https://bug-bash.innovations-sandbox.sonatype.dev"
# BASE_URL = "http://localhost:7777"

# CAMPAIGN_NAME = "cncf"
CAMPAIGN_NAME = "cncf-2021-10-11"

# CSV_COLUMN_NAME_GITHUB_ID = "GitHub ID"
CSV_COLUMN_NAME_GITHUB_ID = "Git Handle"
CSV_COLUMN_NAME_DISPLAY_NAME = "Full Name"

default_gh_username = 'set_your_github_username_env'
username = os.getenv('GITHUB_USERNAME', default_gh_username)
token = os.getenv('GITHUB_TOKEN', '...')
# create a re-usable session object with the user creds in-built
gh_session = requests.Session()
gh_session.auth = (username, token)


def get_github_user(user):
    url = f"https://api.github.com/users/{user}"
    res = gh_session.get(url.format(user))
    if res.status_code == 200:
        return res.json()
    else:
        print(f"\n*** invalid github user: {user}. status code: {r.status_code} ***")
        if r.status_code == 500 and username == default_gh_username:
            print(f"\n*** did you forget to set the GITHUB_USERNAME environment variable? ***\n")
        return


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
    summary_error_message = []
    for participant in rows:

        # detect email instead of required github ID
        if "@" in participant[CSV_COLUMN_NAME_GITHUB_ID]:
            msg = f"\n*** invalid Github ID detected. Skipping participant. participant: {participant}. ***\n"
            summary_error_message.append(msg)
            print(msg)
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
            print(f"Participant already exists, skipping. participant: {participant}")
        else:
            gh_user = get_github_user(participant[CSV_COLUMN_NAME_GITHUB_ID])
            if gh_user is None:
                if username == default_gh_username:
                    msg = f"\n*** did you forget to set the GITHUB_USERNAME environment variable? ***"
                    summary_error_message.append(msg)
                msg = f"\n*** non-existent github id, error. participant: {participant} ***\n"
                summary_error_message.append(msg)
                count_error += 1
                exitCode = 1
                continue

            print(f"Adding participant: {participant[CSV_COLUMN_NAME_GITHUB_ID]} participant: {participant}")
            r = requests.request('PUT', f"{BASE_URL}/participant/add", json=send)
            print(r)
            print(r.text)
            if r.status_code != 200 and r.status_code != 201:
                msg = f"\n*** error adding participant. status code: {r.status_code}, participant: {participant} ***\n"
                summary_error_message.append(msg)
                count_error += 1
            else:
                count_added += 1

    print(f"done processing {len(rows)} participants")
    msg_summary = "\n".join(summary_error_message)
    print(f"errors: {msg_summary}")
    print(f"added {count_added}")
    print(f"skipped {count_skipped}")
    print(f"errored {count_error}")
    exit(exitCode)
