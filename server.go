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
	"github.com/sonatype-nexus-community/bbash/buildversion"
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

type scoringAlert struct {
	RecentHits []string `json:"recent_hits"` // encoded scoring message
}

type scoringMessage struct {
	RepoOwner   string         `json:"repositoryOwner"`
	RepoName    string         `json:"repositoryName"`
	TriggerUser string         `json:"triggerUser"`
	TotalFixed  int            `json:"fixed-bugs"`
	BugCounts   map[string]int `json:"fixed-bug-types"`
	PullRequest int            `json:"pullRequestId"`
}

type leaderboardItem struct {
	UserName string `json:"name"`
	Slug     string `json:"slug"`
	Score    int    `json:"score"`
	Archived bool   `json:"_archived"`
	Draft    bool   `json:"_draft"`
}

type leaderboardPayload struct {
	Fields leaderboardItem `json:"fields"`
}

type leaderboardResponse struct {
	Id string `json:"_id"`
}

const (
	ParamGithubName   string = "gitHubName"
	ParamCampaignName string = "campaignName"
	ParamTeamName     string = "teamName"
	ParamBugCategory  string = "bugCategory"
	ParamPointValue   string = "pointValue"
	Participant       string = "/participant"
	Detail            string = "/detail"
	List              string = "/list"
	Update            string = "/update"
	Delete            string = "/delete"
	Team              string = "/team"
	Add               string = "/add"
	Person            string = "/person"
	Bug               string = "/bug"
	Campaign          string = "/campaign"
	ScoreEvent        string = "/scoring"
	New               string = "/new"
	WebflowApiBase    string = "https://api.webflow.com"
)

// variable allows for changes to url for testing
var webflowBaseAPI = WebflowApiBase
var webflowToken string
var webflowCollection string

func main() {

	e := echo.New()
	e.Debug = true
	e.Logger.SetLevel(log.INFO)

	buildInfoMessage := fmt.Sprintf("BuildVersion: %s, BuildTime: %s, BuildCommit: %s",
		buildversion.BuildVersion, buildversion.BuildTime, buildversion.BuildCommit)
	e.Logger.Infof(buildInfoMessage)
	fmt.Println(buildInfoMessage)

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
		return c.String(http.StatusOK, fmt.Sprintf("I am ALIVE. %s", buildInfoMessage))
	})

	// Participant related endpoints and group

	participantGroup := e.Group(Participant)

	participantGroup.GET(
		fmt.Sprintf("%s/:%s", Detail, ParamGithubName),
		getParticipantDetail).Name = "participant-detail"

	participantGroup.GET(
		fmt.Sprintf("%s/:%s", List, ParamCampaignName),
		getParticipantsList).Name = "participant-list"

	participantGroup.POST(Update, updateParticipant).Name = "participant-update"
	participantGroup.PUT(Add, addParticipant).Name = "participant-add"
	participantGroup.DELETE(
		fmt.Sprintf("%s/:%s", Delete, ParamGithubName),
		deleteParticipant,
	)

	// Team related endpoints and group

	teamGroup := e.Group(Team)

	teamGroup.PUT(Add, addTeam)
	teamGroup.PUT(fmt.Sprintf("%s/:%s/:%s", Person, ParamGithubName, ParamTeamName), addPersonToTeam)

	// Bug related endpoints and group

	bugGroup := e.Group(Bug)

	bugGroup.PUT(Add, addBug)
	bugGroup.POST(fmt.Sprintf("%s/:%s/:%s", Update, ParamBugCategory, ParamPointValue), updateBug)
	bugGroup.GET(List, getBugs)
	bugGroup.PUT(List, putBugs)

	// Campaign related endpoints and group

	campaignGroup := e.Group(Campaign)

	campaignGroup.PUT(fmt.Sprintf("%s/:%s", Add, ParamCampaignName), addCampaign)

	// Scoring related endpoints and group
	scoreGroup := e.Group(ScoreEvent)

	scoreGroup.POST(New, logNewScore)

	routes := e.Routes()

	for _, v := range routes {
		fmt.Printf("Registered route: %s %s as %s\n", v.Method, v.Path, v.Name)
	}

	err = e.Start(":7777")
	if err != nil {
		e.Logger.Error(err)
	}
}

type ParticipantCreateError struct {
	Status string
}

func (e *ParticipantCreateError) Error() string {
	return fmt.Sprintf("could not create upstream participant. response status: %s", e.Status)
}

func upstreamNewParticipant(c echo.Context, p participant) (id string, err error) {
	item := leaderboardItem{}
	item.UserName = p.GitHubName
	item.Slug = p.GitHubName
	item.Score = 0
	item.Archived = false
	item.Draft = false

	payload := leaderboardPayload{}
	payload.Fields = item

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/collections/%s/items?live=true", webflowBaseAPI, webflowCollection), bytes.NewReader(body))
	if err != nil {
		return
	}
	requestHeaderSetup(req)

	res, err := getNetClient().Do(req)
	if err != nil {
		return
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		c.Logger().Debug(req)
		var responseBody []byte
		_, _ = res.Body.Read(responseBody)
		c.Logger().Debug(responseBody)
		// return a real error to the caller to indicate failure
		err = &ParticipantCreateError{res.Status}
		if errContext := c.String(http.StatusInternalServerError, err.Error()); errContext != nil {
			err = errContext
		}
		return
	}

	var response leaderboardResponse
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return
	}
	id = response.Id
	return
}

// see: https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
func getNetClient() (netClient *http.Client) {
	netClient = &http.Client{
		Timeout: time.Second * 10,
	}
	return
}

func requestHeaderSetup(req *http.Request) {
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", webflowToken))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("accept-version", "1.0.0")
}

type ParticipantUpdateError struct {
	Status string
}

func (e *ParticipantUpdateError) Error() string {
	return fmt.Sprintf("could not update score. response status: %s", e.Status)
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

	url := fmt.Sprintf("%s/collections/%s/items/%s?live=true", webflowBaseAPI, webflowCollection, webflowId)
	req, err := http.NewRequest("PATCH", url, bytes.NewReader(body))
	if err != nil {
		return
	}
	requestHeaderSetup(req)

	res, err := getNetClient().Do(req)
	if err != nil {
		return
	} else if res.StatusCode < 200 || res.StatusCode >= 300 {
		c.Logger().Debug(req)
		var responseBody []byte
		_, _ = res.Body.Read(responseBody)
		c.Logger().Debug(responseBody)
		// return a real error to the caller to indicate failure
		err = &ParticipantUpdateError{res.Status}
		if errContext := c.String(http.StatusInternalServerError, err.Error()); errContext != nil {
			err = errContext
		}
	}

	return
}

var ParticipatingOrgs = map[string]bool{
	"thanos-io":                true,
	"serverlessworkflow":       true,
	"chaos-mesh":               true,
	"cri-o":                    true,
	"openebs":                  true,
	"buildpacks":               true,
	"schemahero":               true,
	"sonatype-nexus-community": true,
}

const sqlSelectParticipantId = `SELECT
		participants.Id
		FROM participants
		WHERE participants.GitHubName = $1`

func validScore(c echo.Context, owner string, user string) bool {
	// check if repo is in participating set
	if !ParticipatingOrgs[owner] {
		c.Logger().Debugf("score not valid, missing ParticipatingOrg. owner: %s, user: %s", owner, user)
		return false
	}

	// Check if participant is registered
	row := db.QueryRow(sqlSelectParticipantId, user)
	var id string
	err := row.Scan(&id)
	if err != nil {
		c.Logger().Debugf("score is not valid due to missing participant. owner: %s, user: %s, err: %v", owner, user, err)
	}
	return err == nil
}

const sqlSelectPointValue = `SELECT pointValue FROM bugs WHERE category = $1`

func scorePoints(c echo.Context, msg scoringMessage) (points int) {
	points = 0
	scored := 0

	for bugType, count := range msg.BugCounts {
		row := db.QueryRow(sqlSelectPointValue, bugType)
		var value = 1
		if err := row.Scan(&value); err != nil {
			// ignore (and clear return) error from scan operation
			c.Logger().Debugf("ignoring missing pointValue. bugType: %s, err: %+v, msg: %+v", bugType, err, msg)
		}

		points += count * value
		scored += count
	}

	// add 1 point for all non-classified fixed bugs
	if scored < msg.TotalFixed {
		points += msg.TotalFixed - scored
	}

	return
}

const sqlUpdateParticipantScore = `UPDATE participants 
		SET Score = Score + $1 
		WHERE GitHubName = $2 
		RETURNING UpstreamId, Score`

func updateParticipantScore(c echo.Context, username string, delta int) (err error) {
	var id string
	var score int
	row := db.QueryRow(sqlUpdateParticipantScore, delta, username)
	err = row.Scan(&id, &score)
	if err != nil {
		return
	}

	err = upstreamUpdateScore(c, id, score)
	return
}

// was not seeing enough detail when newScore() returns error, so capturing such cases in the log.
func logNewScore(c echo.Context) (err error) {
	if err = newScore(c); err != nil {
		c.Logger().Errorf("error calling newScore. err: %+v", err)
	}
	return
}

const sqlScoreQuery = `SELECT points
			FROM scoring_events
			WHERE repoOwner = $1
				AND repoName = $2
				AND pr = $3`

const sqlInsertScoringEvent = `INSERT INTO scoring_events
			(repoOwner, repoName, pr, username, points)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (repoOwner, repoName, pr) DO
				UPDATE SET points = $5`

func newScore(c echo.Context) (err error) {
	var alert scoringAlert
	err = json.NewDecoder(c.Request().Body).Decode(&alert)
	if err != nil {
		return
	}

	//c.Logger().Debugf("scoringAlert: %+v", alert)

	for _, rawMsg := range alert.RecentHits {
		var msg scoringMessage
		err = json.Unmarshal([]byte(rawMsg), &msg)
		if err != nil {
			c.Logger().Debugf("error unmarshalling scoringMessage, err: %+v, rawMsg: %s", err, rawMsg)
			return
		}

		// if this particular entry is not valid, ignore it and continue processing
		if !validScore(c, msg.RepoOwner, msg.TriggerUser) {
			continue
		}

		newPoints := scorePoints(c, msg)

		var tx *sql.Tx
		tx, err = db.Begin()
		if err != nil {
			return
		}

		row := db.QueryRow(sqlScoreQuery, msg.RepoOwner, msg.RepoName, msg.PullRequest)
		oldPoints := 0
		err = row.Scan(&oldPoints)
		if err != nil {
			// ignore error case from scan when no row exists, will occur when this is a new score event
			c.Logger().Debugf("ignoring likely new score event. err: %+v, scoringMessage: %+v", err, msg)
		}

		_, err = db.Exec(sqlInsertScoringEvent, msg.RepoOwner, msg.RepoName, msg.PullRequest, msg.TriggerUser, newPoints)
		if err != nil {
			return
		}

		err = updateParticipantScore(c, msg.TriggerUser, newPoints-oldPoints)
		if err != nil {
			return
		}

		err = tx.Commit()
		if err != nil {
			return
		}

		c.Logger().Debugf("score updated. scoringMessage: %+v", msg)
	}

	//c.Logger().Debugf("scoringAlert completed: %+v", alert)

	return c.NoContent(http.StatusAccepted)
}

const sqlSelectParticipantDetail = `SELECT 
		participants.Id, GitHubName, Email, DisplayName, Score, teams.TeamName, JoinedAt, campaigns.CampaignName 
		FROM participants
		LEFT JOIN teams ON teams.Id = participants.fk_team
		INNER JOIN campaigns ON campaigns.Id = participants.Campaign
		WHERE participants.GitHubName = $1`

func getParticipantDetail(c echo.Context) (err error) {
	gitHubName := c.Param(ParamGithubName)
	c.Logger().Debug("Getting detail for ", gitHubName)

	row := db.QueryRow(sqlSelectParticipantDetail, gitHubName)

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

const sqlSelectParticipantsByCampaign = `SELECT
		participants.Id, GitHubName, Email, DisplayName, Score, teams.TeamName, JoinedAt, campaigns.CampaignName 
		FROM participants
		LEFT JOIN teams ON participants.fk_team = teams.Id
		INNER JOIN campaigns ON participants.Campaign = campaigns.Id
		WHERE campaigns.CampaignName = $1`

func getParticipantsList(c echo.Context) (err error) {
	campaignName := c.Param(ParamCampaignName)
	c.Logger().Debug("Getting list for ", campaignName)

	rows, err := db.Query(sqlSelectParticipantsByCampaign, campaignName)
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

const sqlUpdateParticipant = `UPDATE participants 
		SET 
		    GithubName = $1,
		    Email = $2,
		    DisplayName = $3,
		    Score = $4,
		    Campaign = (SELECT Id FROM campaigns WHERE CampaignName = $5),
		    fk_team = $6		    
		WHERE Id = $7`

func updateParticipant(c echo.Context) (err error) {
	participant := participant{}

	err = json.NewDecoder(c.Request().Body).Decode(&participant)
	if err != nil {
		return
	}

	res, err := db.Exec(
		sqlUpdateParticipant,
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

const sqlDeleteParticipant = `DELETE FROM participants WHERE GithubName = $1`

func deleteParticipant(c echo.Context) (err error) {
	githubName := c.Param(ParamGithubName)
	_, err = db.Exec(sqlDeleteParticipant, githubName)
	return
}

const sqlInsertParticipant = `INSERT INTO participants 
		(GithubName, Email, DisplayName, Score, UpstreamId, Campaign) 
		VALUES ($1, $2, $3, $4, $5, (SELECT Id FROM campaigns WHERE CampaignName = $6))
		RETURNING Id, Score, JoinedAt`

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

	var guid string
	err = db.QueryRow(
		sqlInsertParticipant,
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

const sqlInsertTeam = `INSERT INTO teams
		(TeamName, Organization)
		VALUES ($1, $2)
		RETURNING Id`

func addTeam(c echo.Context) (err error) {
	team := team{}

	err = json.NewDecoder(c.Request().Body).Decode(&team)
	if err != nil {
		return
	}

	var guid string
	err = db.QueryRow(
		sqlInsertTeam,
		team.TeamName,
		team.Organization).Scan(&guid)
	if err != nil {
		return
	}

	return c.String(http.StatusCreated, guid)
}

const sqlUpdateParticipantTeam = `UPDATE participants 
		SET fk_team = (SELECT Id FROM teams WHERE TeamName = $1)
		WHERE GitHubName = $2`

func addPersonToTeam(c echo.Context) (err error) {
	teamName := c.Param(ParamTeamName)
	gitHubName := c.Param(ParamGithubName)

	if teamName == "" || gitHubName == "" {
		return c.NoContent(http.StatusBadRequest)
	}

	res, err := db.Exec(
		sqlUpdateParticipantTeam,
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

const sqlInsertBug = `INSERT INTO bugs
		(category, pointValue)
		VALUES ($1, $2)
		RETURNING ID`

func addBug(c echo.Context) (err error) {
	bug := bug{}

	err = json.NewDecoder(c.Request().Body).Decode(&bug)
	if err != nil {
		return
	}

	var guid string
	err = db.QueryRow(sqlInsertBug, bug.Category, bug.PointValue).Scan(&guid)
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

const sqlUpdateBug = `UPDATE bugs
		SET pointValue = $1
		WHERE category = $2`

func updateBug(c echo.Context) (err error) {

	category := c.Param(ParamBugCategory)
	pointValue, err := strconv.Atoi(c.Param(ParamPointValue))
	if err != nil {
		return
	}

	c.Logger().Debug(category)

	res, err := db.Exec(sqlUpdateBug, pointValue, category)
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

const sqlSelectBug = `SELECT * FROM bugs`

func getBugs(c echo.Context) (err error) {

	rows, err := db.Query(sqlSelectBug)
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
	var inserted []bug
	for _, bug := range bugs {
		err = db.QueryRow(sqlInsertBug, bug.Category, bug.PointValue).Scan(&bug.Id)
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

const sqlInsertCampaign = `INSERT INTO campaigns 
		(CampaignName) 
		VALUES ($1)
		RETURNING Id`

func addCampaign(c echo.Context) (err error) {
	campaignName := strings.TrimSpace(c.Param(ParamCampaignName))
	if len(campaignName) == 0 {
		err = fmt.Errorf("invalid parameter %s: %s", ParamCampaignName, campaignName)
		c.Logger().Error(err)

		return c.String(http.StatusBadRequest, err.Error())
	}

	var guid string
	err = db.QueryRow(
		sqlInsertCampaign,
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
