//
// Copyright (c) 2021-present Sonatype, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

//go:build go1.16
// +build go1.16

package types

import (
	"database/sql"
	"time"
)

type SourceControlProviderStruct struct {
	ID      string `json:"guid"`
	SCPName string `json:"scpName"`
	Url     string `json:"url"`
}

type CampaignStruct struct {
	ID           string         `json:"guid"`
	Name         string         `json:"name"`
	CreatedOn    time.Time      `json:"createdOn"`
	CreatedOrder int            `json:"createdOrder"`
	StartOn      time.Time      `json:"startOn"`
	EndOn        time.Time      `json:"endOn"`
	Note         sql.NullString `json:"note"`
}

type OrganizationStruct struct {
	ID           string `json:"guid"`
	SCPName      string `json:"scpName"`
	Organization string `json:"organization"`
}

type ScoringMessage struct {
	EventSource string         `json:"eventSource"`
	RepoOwner   string         `json:"repositoryOwner"`
	RepoName    string         `json:"repositoryName"`
	TriggerUser string         `json:"triggerUser"`
	TotalFixed  int            `json:"fixed-bugs"`
	BugCounts   map[string]int `json:"fixed-bug-types"`
	PullRequest int            `json:"pullRequestId"`
}

type ParticipantStruct struct {
	ID           string    `json:"guid"`
	CampaignName string    `json:"campaignName"`
	ScpName      string    `json:"scpName"`
	LoginName    string    `json:"loginName"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"displayName"`
	Score        int       `json:"score"`
	TeamName     string    `json:"teamName"`
	JoinedAt     time.Time `json:"joinedAt"`
}

type TeamStruct struct {
	Id           string `json:"guid"`
	CampaignName string `json:"campaignName"`
	Name         string `json:"name"`
}

type BugStruct struct {
	Id         string `json:"guid"`
	Campaign   string `json:"campaign"`
	Category   string `json:"category"`
	PointValue int    `json:"pointValue"`
}

type Poll struct {
	Id                string    `json:"pollInstance"`
	LastPolled        time.Time `json:"lastPolledOn"`
	EnvBaseTime       time.Time `json:"envBaseTime"`
	LastPollCompleted time.Time `json:"lastPollCompleted"`
}
