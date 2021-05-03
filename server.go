//
// Copyright 2021-present Sonatype Inc.
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

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
)

var db *sql.DB

type participant struct {
	ID           string `json:"guid"`
	GitHubName   string `json:"gitHubName"`
	Email        string `json:"email"`
	DisplayName  string `json:"displayName"`
	Score        int    `json:"score"`
	fkTeam       sql.NullString
	JoinedAt     time.Time `json:"joinedAt"`
	CampaignName string    `json:"campaignName"`
}

type team struct {
	Id           string         `json:"guid"`
	TeamName     string         `json:"teamName"`
	Organization sql.NullString `json:"organization"`
}

type creationResponse struct {
	Id        string                 `json:"guid"`
	Endpoints map[string]interface{} `json:"endpoints"`
	Object    interface{}            `json:"object"`
}

type endpointDetail struct {
	URI  string `json:"uri"`
	Verb string `json:"httpVerb"`
}

type bug struct {
	Id         string `json:"guid"`
	Category   string `json:"category"`
	PointValue int    `json:"pointValue"`
}

type scoring_alert struct {
	RecentHits []string `json:"recent_hits"` // encoded scoring message
}

type scoring_message struct {
	RepoOwner   string         `json:"repositoryOwner"`
	RepoName    string         `json:"repositoryName"`
	TriggerUser string         `json:"triggerUser"`
	TotalFixed  int            `json:"fixed-bugs"`
	BugCounts   map[string]int `json:"fixed-bug-types"`
	PullRequest int            `json:"pullRequestId"`
}

type leaderboard_item struct {
	UserName string `json:"name"`
	Slug     string `json:"slug"`
	Score    int    `json:"score"`
	Archived bool   `json:"_archived"`
	Draft    bool   `json:"_draft"`
}

type leaderboard_payload struct {
	Fields leaderboard_item `json:"fields"`
}

type leaderboard_response struct {
	Id string `json:"_id"`
}

const (
	PARAM_ID            string = "id"
	PARAM_GITHUB_NAME   string = "gitHubName"
	PARAM_CAMPAIGN_NAME string = "campaignName"
	PARAM_TEAM_NAME     string = "teamName"
	PARAM_BUG_CATEGORY  string = "bugCategory"
	PARAM_POINT_VALUE   string = "pointValue"
	PARTICIPANT         string = "/participant"
	DETAIL              string = "/detail"
	LIST                string = "/list"
	UPDATE              string = "/update"
	DELETE              string = "/delete"
	TEAM                string = "/team"
	ADD                 string = "/add"
	PERSON              string = "/person"
	BUG                 string = "/bug"
	CAMPAIGN            string = "/campaign"
	SCORE_EVENT         string = "/scoring"
	WEBFLOW_API_BASE    string = "https://api.webflow.com"
)

var webflowToken string
var webflowCollection string

func main() {

	e := echo.New()
	e.Debug = true
	e.Logger.SetLevel(log.INFO)

	err := godotenv.Load(".env")
	if err != nil {
		e.Logger.Error(err)
	}

	host := os.Getenv("PG_HOST")
	port, _ := strconv.Atoi(os.Getenv("PG_PORT"))
	user := os.Getenv("PG_USERNAME")
	password := os.Getenv("PG_PASSWORD")
	dbname := os.Getenv("PG_DB_NAME")
	sslMode := os.Getenv("SSL_MODE")
	webflowToken = os.Getenv("WEBFLOW_TOKEN")
	webflowCollection = os.Getenv("WEBFLOW_COLLECTION_ID")

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslMode)
	db, err = sql.Open("postgres", psqlInfo)
	if err != nil {
		e.Logger.Error(err)
	}
	defer func() {
		_ = db.Close()
	}()

	err = db.Ping()
	if err != nil {
		e.Logger.Error(err)
	}

	err = migrateDB(db)
	if err != nil {
		e.Logger.Error(err)
	}

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "I am ALIVE")
	})

	// Participant related endpoints and group

	participantGroup := e.Group(PARTICIPANT)

	participantGroup.GET(
		fmt.Sprintf("%s/:%s", DETAIL, PARAM_GITHUB_NAME),
		getParticipantDetail).Name = "participant-detail"

	participantGroup.GET(
		fmt.Sprintf("%s/:%s", LIST, PARAM_CAMPAIGN_NAME),
		getParticipantsList).Name = "participant-list"

	participantGroup.POST(UPDATE, updateParticipant).Name = "participant-update"
	participantGroup.PUT(ADD, addParticipant).Name = "participant-add"
	participantGroup.DELETE(
		fmt.Sprintf("%s/:%s", DELETE, PARAM_GITHUB_NAME),
		deleteParticipant,
	)

	// Team related endpoints and group

	teamGroup := e.Group(TEAM)

	teamGroup.PUT(ADD, addTeam)
	teamGroup.PUT(fmt.Sprintf("%s/:%s/:%s", PERSON, PARAM_GITHUB_NAME, PARAM_TEAM_NAME), addPersonToTeam)

	// Bug related endpoints and group

	bugGroup := e.Group(BUG)

	bugGroup.PUT(ADD, addBug)
	bugGroup.POST(fmt.Sprintf("%s/:%s/:%s", UPDATE, PARAM_BUG_CATEGORY, PARAM_POINT_VALUE), updateBug)
	bugGroup.GET(LIST, getBugs)
	bugGroup.PUT(LIST, putBugs)

	// Campaign related endpoints and group

	campaignGroup := e.Group(CAMPAIGN)

	campaignGroup.PUT(fmt.Sprintf("%s/:%s", ADD, PARAM_CAMPAIGN_NAME), addCampaign)

	// Scoreing related endpoints and group
	scoreGroup := e.Group(SCORE_EVENT)

	scoreGroup.POST("/new", newScore)

	routes := e.Routes()

	for _, v := range routes {
		fmt.Printf("Registered route: %s %s as %s\n", v.Method, v.Path, v.Name)
	}

	err = e.Start(":7777")
	if err != nil {
		e.Logger.Error(err)
	}
}

func upstreamNewParticipant(c echo.Context, p participant) (id string, err error) {
	item := leaderboard_item{}
	item.UserName = p.GitHubName
	item.Slug = p.GitHubName
	item.Score = 0
	item.Archived = false
	item.Draft = false

	payload := leaderboard_payload{}
	payload.Fields = item

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/collections/%s/items?live=true", WEBFLOW_API_BASE, webflowCollection), bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", webflowToken))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("accept-version", "1.0.0")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		c.Logger().Debug(req)
		var responseBody []byte
		res.Body.Read(responseBody)
		c.Logger().Debug(responseBody)
		err = c.String(http.StatusInternalServerError, "Could not create upstream participant")
		return
	}

	var response leaderboard_response
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return
	}
	id = response.Id
	return
}

func upstreamUpdateScore(c echo.Context, webflowId string, score int) (err error) {

	var payload struct {
		Fields struct {
			Score int `json:"score"`
		} `json:"fields"`
	}
	payload.Fields.Score = score

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	url := fmt.Sprintf("%s/collections/%s/items/%s?live=true", WEBFLOW_API_BASE, webflowCollection, webflowId)
	req, err := http.NewRequest("PATCH", url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", webflowToken))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("accept-version", "1.0.0")

	res, err := http.DefaultClient.Do(req)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		c.Logger().Debug(req)
		var responseBody []byte
		res.Body.Read(responseBody)
		c.Logger().Debug(responseBody)
		err = c.String(http.StatusInternalServerError, "Could not update score")
	}

	return
}

var PARTICIPATING_ORGS = map[string]bool{
	"thanos-io":          true,
	"serverlessworkflow": true,
	"chaos-mesh":         true,
	"cri-o":              true,
	"openebs":            true,
	"buildpacks":         true,
	"schemahero":         true,
}

func validScore(owner string, name string, user string) bool {
	// check if repo is in participating set
	if !PARTICIPATING_ORGS[owner] {
		return false
	}

	// Check if participant is registered
	sqlQuery := `SELECT
		participants.Id
		FROM participants
		WHERE participants.GitHubName = $1`
	row := db.QueryRow(sqlQuery, user)
	var id string
	err := row.Scan(&id)
	if err != nil {
		return false
	}

	return true
}

func scorePoints(msg scoring_message) (points int) {
	points = 0
	scored := 0

	for bugType, count := range msg.BugCounts {
		sqlQuery := `SELECT pointValue FROM bugs WHERE category = $1`
		row := db.QueryRow(sqlQuery, bugType)
		var value int = 1
		row.Scan(&value)
		points += count * value
		scored += count
	}

	// add 1 point for all non-classified fixed bugs
	if scored < msg.TotalFixed {
		points += msg.TotalFixed - scored
	}

	return
}

func updateParticipantScore(c echo.Context, username string, delta int) (err error) {
	sqlQuery := `UPDATE participants SET Score = Score + $1 WHERE GitHubName = $2 RETURNING UpstreamId, Score`
	var id string
	var score int
	row := db.QueryRow(sqlQuery, delta, username)
	err = row.Scan(&id, &score)
	if err != nil {
		return
	}

	err = upstreamUpdateScore(c, id, score)
	return
}

func newScore(c echo.Context) (err error) {
	var alert scoring_alert
	err = json.NewDecoder(c.Request().Body).Decode(&alert)
	if err != nil {
		return
	}

	c.Logger().Debug(alert)

	for _, rawMsg := range alert.RecentHits {
		var msg scoring_message
		err = json.Unmarshal([]byte(rawMsg), &msg)
		if err != nil {
			return
		}
		c.Logger().Debug(msg)

		// if this particular entry is not valid, ignore it and continue processing
		if !validScore(msg.RepoOwner, msg.RepoName, msg.TriggerUser) {
			c.Logger().Debug("Score is not valid!")
			continue
		}

		newPoints := scorePoints(msg)

		tx, err := db.Begin()
		if err != nil {
			return err
		}

		scoreQuery := `SELECT points
			FROM scoring_events
			WHERE repoOwner = $1
				AND repoName = $2
				AND pr = $3`

		row := db.QueryRow(scoreQuery, msg.RepoOwner, msg.RepoName, msg.PullRequest)
		oldPoints := 0
		err = row.Scan(&oldPoints)
		if err != nil {
			c.Logger().Debug(err)
		}

		insertEvent := `INSERT INTO scoring_events
			(repoOwner, repoName, pr, username, points)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (repoOwner, repoName, pr) DO
				UPDATE SET points = $5`

		_, err = db.Exec(insertEvent, msg.RepoOwner, msg.RepoName, msg.PullRequest, msg.TriggerUser, newPoints)
		if err != nil {
			c.Logger().Debug(err)
			return err
		}

		err = updateParticipantScore(c, msg.TriggerUser, newPoints-oldPoints)
		if err != nil {
			c.Logger().Debug(err)
			return err
		}

		err = tx.Commit()
		if err != nil {
			c.Logger().Debug(err)
			return err
		}
	}

	return c.NoContent(http.StatusAccepted)
}

func getParticipantDetail(c echo.Context) (err error) {
	gitHubName := c.Param(PARAM_GITHUB_NAME)
	c.Logger().Debug("Getting detail for ", gitHubName)

	sqlQuery := `SELECT 
		participants.Id, GitHubName, Email, DisplayName, Score, teams.TeamName, JoinedAt, campaigns.CampaignName 
		FROM participants
		LEFT JOIN teams ON teams.Id = participants.fk_team
		INNER JOIN campaigns ON campaigns.Id = participants.Campaign
		WHERE participants.GitHubName = $1`

	row := db.QueryRow(sqlQuery, gitHubName)

	participant := new(participant)
	err = row.Scan(&participant.ID,
		&participant.GitHubName,
		&participant.Email,
		&participant.DisplayName,
		&participant.Score,
		&participant.fkTeam,
		&participant.JoinedAt,
		&participant.CampaignName,
	)

	if err != nil {
		c.Logger().Error(err)
		return
	}

	return c.JSON(http.StatusOK, participant)
}

func getParticipantsList(c echo.Context) (err error) {
	campaignName := c.Param(PARAM_CAMPAIGN_NAME)
	c.Logger().Debug("Getting list for ", campaignName)

	sqlQuery := `SELECT
		participants.Id, GitHubName, Email, DisplayName, Score, teams.TeamName, JoinedAt, campaigns.CampaignName 
		FROM participants
		LEFT JOIN teams ON participants.fk_team = teams.Id
		INNER JOIN campaigns ON participants.Campaign = campaigns.Id
		WHERE campaigns.CampaignName = $1`

	rows, err := db.Query(sqlQuery, campaignName)
	if err != nil {
		return
	}

	var participants []participant
	for rows.Next() {
		participant := new(participant)
		err = rows.Scan(
			&participant.ID,
			&participant.GitHubName,
			&participant.Email,
			&participant.DisplayName,
			&participant.Score,
			&participant.fkTeam,
			&participant.JoinedAt,
			&participant.CampaignName,
		)
		if err != nil {
			return
		}
		participants = append(participants, *participant)
	}

	return c.JSON(http.StatusOK, participants)
}

func updateParticipant(c echo.Context) (err error) {
	participant := participant{}

	err = json.NewDecoder(c.Request().Body).Decode(&participant)
	if err != nil {
		return
	}

	sqlUpdate := `UPDATE participants 
		SET 
		    GithubName = $1,
		    Email = $2,
		    DisplayName = $3,
		    Score = $4,
		    Campaign = (SELECT Id FROM campaigns WHERE CampaignName = $5),
		    fk_team = $6		    
		WHERE Id = $7`

	res, err := db.Exec(
		sqlUpdate,
		participant.GitHubName,
		participant.Email,
		participant.DisplayName,
		participant.Score,
		participant.CampaignName,
		participant.fkTeam,
		participant.ID,
	)
	if err != nil {
		return
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return
	}

	err = updateParticipantScore(c, participant.GitHubName, 0)
	if err != nil {
		return
	}

	if rows == 1 {
		c.Logger().Infof(
			"Success, huzzah! Participant updated, ID: %s, gitHubName: %s",
			participant.ID,
			participant.GitHubName,
		)

		return c.NoContent(http.StatusNoContent)
	} else {
		c.Logger().Errorf(
			"No row was updated, something goofy has occurred, ID: %s, gitHubName: %s, rows: %s",
			participant.ID,
			participant.GitHubName,
			rows,
		)

		return c.NoContent(http.StatusBadRequest)
	}
}

func deleteParticipant(c echo.Context) (err error) {
	githubName := c.Param(PARAM_GITHUB_NAME)

	sqlDelete := `DELETE FROM participants WHERE GithubName = $1`

	_, err = db.Exec(sqlDelete, githubName)
	return
}

func addParticipant(c echo.Context) (err error) {
	participant := participant{}

	err = json.NewDecoder(c.Request().Body).Decode(&participant)
	if err != nil {
		return
	}

	webflowId, err := upstreamNewParticipant(c, participant)
	if err != nil {
		return
	}

	sqlInsert := `INSERT INTO participants 
		(GithubName, Email, DisplayName, Score, UpstreamId, Campaign) 
		VALUES ($1, $2, $3, $4, $5, (SELECT Id FROM campaigns WHERE CampaignName = $6))
		RETURNING Id, Score, JoinedAt`

	var guid string
	err = db.QueryRow(
		sqlInsert,
		participant.GitHubName,
		participant.Email,
		participant.DisplayName,
		0,
		webflowId,
		participant.CampaignName).Scan(&guid, &participant.Score, &participant.JoinedAt)
	if err != nil {
		return
	}

	participant.ID = guid

	detailUri := c.Echo().Reverse("participant-detail", participant.GitHubName)
	updateUri := c.Echo().Reverse("participant-update")
	endpoints := make(map[string]interface{})
	endpoints["participantDetail"] = endpointDetail{URI: detailUri, Verb: "GET"}
	endpoints["participantUpdate"] = endpointDetail{URI: updateUri, Verb: "PUT"}

	creation := creationResponse{
		Id:        guid,
		Endpoints: endpoints,
		Object:    participant,
	}

	return c.JSON(http.StatusCreated, creation)
}

func addTeam(c echo.Context) (err error) {
	team := team{}

	err = json.NewDecoder(c.Request().Body).Decode(&team)
	if err != nil {
		return
	}

	sqlInsert := `INSERT INTO teams
		(TeamName, Organization)
		VALUES ($1, $2)
		RETURNING Id`

	var guid string
	err = db.QueryRow(
		sqlInsert,
		team.TeamName,
		team.Organization).Scan(&guid)
	if err != nil {
		return
	}

	return c.String(http.StatusCreated, guid)
}

func addPersonToTeam(c echo.Context) (err error) {
	teamName := c.Param(PARAM_TEAM_NAME)
	gitHubName := c.Param(PARAM_GITHUB_NAME)

	if teamName == "" || gitHubName == "" {
		return c.NoContent(http.StatusBadRequest)
	}

	sqlUpdate := `UPDATE participants 
		SET fk_team = (SELECT Id FROM teams WHERE TeamName = $1)
		WHERE GitHubName = $2`

	res, err := db.Exec(
		sqlUpdate,
		teamName,
		gitHubName)
	if err != nil {
		return
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return
	}

	if rows > 0 {
		c.Logger().Infof(
			"Success, huzzah! Row updated, teamName: %s, gitHubName: %s",
			teamName,
			gitHubName,
		)

		return c.NoContent(http.StatusNoContent)
	} else {
		c.Logger().Errorf(
			"No row was updated, something goofy has occurred, teamName: %s, gitHubName: %s",
			teamName,
			gitHubName,
		)

		return c.NoContent(http.StatusBadRequest)
	}
}

func addBug(c echo.Context) (err error) {
	bug := bug{}

	err = json.NewDecoder(c.Request().Body).Decode(&bug)
	if err != nil {
		return
	}

	sqlInsert := `INSERT INTO bugs
		(category, pointValue)
		VALUES ($1, $2)
		RETURNING ID`

	var guid string
	err = db.QueryRow(sqlInsert, bug.Category, bug.PointValue).Scan(&guid)
	if err != nil {
		return
	}
	bug.Id = guid
	creation := creationResponse{
		Id:     guid,
		Object: bug,
	}
	return c.JSON(http.StatusCreated, creation)
}

func updateBug(c echo.Context) (err error) {

	category := c.Param(PARAM_BUG_CATEGORY)
	pointValue, err := strconv.Atoi(c.Param(PARAM_POINT_VALUE))
	if err != nil {
		return
	}

	c.Logger().Debug(category)

	sqlUpdate := `UPDATE bugs
		SET pointValue = $1
		WHERE category = $2`
	res, err := db.Exec(sqlUpdate, pointValue, category)
	if err != nil {
		return
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return
	}
	if rows < 1 {
		return c.String(http.StatusNotFound, "Bug Category not found")
	}

	return c.String(http.StatusOK, "Success")
}

func getBugs(c echo.Context) (err error) {

	sqlQuery := `SELECT * FROM bugs`

	rows, err := db.Query(sqlQuery)
	if err != nil {
		return
	}

	var bugs []bug
	for rows.Next() {
		bug := bug{}
		err = rows.Scan(&bug.Id, &bug.Category, &bug.PointValue)
		if err != nil {
			return
		}
		bugs = append(bugs, bug)
	}

	return c.JSON(http.StatusOK, bugs)
}

func putBugs(c echo.Context) (err error) {
	var bugs []bug
	err = json.NewDecoder(c.Request().Body).Decode(&bugs)
	if err != nil {
		return
	}

	tx, err := db.Begin()
	if err != nil {
		return
	}
	sqlInsert := `INSERT INTO bugs
		(category, pointValue)
		VALUES ($1, $2)
		RETURNING ID`
	var inserted []bug
	for _, bug := range bugs {
		err = db.QueryRow(sqlInsert, bug.Category, bug.PointValue).Scan(&bug.Id)
		if err != nil {
			return
		}
		inserted = append(inserted, bug)
	}
	err = tx.Commit()
	if err != nil {
		return
	}

	response := creationResponse{
		Id:     inserted[0].Id,
		Object: inserted,
	}

	return c.JSON(http.StatusCreated, response)
}

func addCampaign(c echo.Context) (err error) {
	campaignName := strings.TrimSpace(c.Param(PARAM_CAMPAIGN_NAME))
	if len(campaignName) == 0 {
		err = fmt.Errorf("invalid parameter %s: %s", PARAM_CAMPAIGN_NAME, campaignName)
		c.Logger().Error(err)

		return c.String(http.StatusBadRequest, err.Error())
	}

	sqlInsert := `INSERT INTO campaigns 
		(CampaignName) 
		VALUES ($1)
		RETURNING Id`

	var guid string
	err = db.QueryRow(
		sqlInsert,
		campaignName).Scan(&guid)
	if err != nil {
		return
	}

	return c.String(http.StatusCreated, guid)
}

func migrateDB(db *sql.DB) (err error) {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://db/migrations",
		"postgres", driver)

	if migrateErrorApplicable(err) {
		return
	}

	if err = m.Up(); err != nil {
		if migrateErrorApplicable(err) {
			return
		} else {
			err = nil
		}
	}

	return
}

func migrateErrorApplicable(err error) bool {
	if err == nil || err == migrate.ErrNoChange {
		return false
	}
	return true
}
