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

type campaignStruct struct {
	ID           string         `json:"guid"`
	Name         string         `json:"name"`
	CreatedOn    time.Time      `json:"createdOn"`
	CreatedOrder int            `json:"createdOrder"`
	Active       bool           `json:"active"`
	Note         sql.NullString `json:"note"`
}

type sourceControlProvider struct {
	ID      string `json:"guid"`
	SCPName string `json:"scpName"`
	Url     string `json:"url"`
}

type organization struct {
	ID           string `json:"guid"`
	SCPName      string `json:"scpName"`
	Organization string `json:"organization"`
}

type participant struct {
	ID           string `json:"guid"`
	CampaignName string `json:"campaignName"`
	ScpName      string `json:"scpName"`
	LoginName    string `json:"loginName"`
	Email        string `json:"email"`
	DisplayName  string `json:"displayName"`
	Score        int    `json:"score"`
	fkTeam       sql.NullString
	JoinedAt     time.Time `json:"joinedAt"`
}

type team struct {
	Id       string `json:"guid"`
	Campaign string `json:"campaign"`
	TeamName string `json:"teamName"`
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
	Campaign   string `json:"campaign"`
	Category   string `json:"category"`
	PointValue int    `json:"pointValue"`
}

type scoringAlert struct {
	RecentHits []string `json:"recent_hits"` // encoded scoring message
}

type scoringMessage struct {
	EventSource string         `json:"eventSource"`
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
	ParamScpName          string = "scpName"
	ParamLoginName        string = "loginName"
	ParamCampaignName     string = "campaignName"
	ParamTeamName         string = "teamName"
	ParamBugCategory      string = "bugCategory"
	ParamPointValue       string = "pointValue"
	ParamOrganizationName string = "organizationName"
	SourceControlProvider string = "/scp"
	Organization          string = "/organization"
	Participant           string = "/participant"
	Detail                string = "/detail"
	List                  string = "/list"
	Update                string = "/update"
	Delete                string = "/delete"
	Team                  string = "/team"
	Add                   string = "/add"
	Person                string = "/person"
	Bug                   string = "/bug"
	Campaign              string = "/campaign"
	ScoreEvent            string = "/scoring"
	New                   string = "/new"
	WebflowApiBase        string = "https://api.webflow.com"
)

// variable allows for changes to url for testing
var webflowBaseAPI = WebflowApiBase
var webflowToken string
var webflowCollection string

const envPGHost = "PG_HOST"
const envPGPort = "PG_PORT"
const envPGUsername = "PG_USERNAME"
const envPGPassword = "PG_PASSWORD"
const envPGDBName = "PG_DB_NAME"
const envSSLMode = "SSL_MODE"

var errRecovered error

func main() {
	e := echo.New()
	e.Debug = true
	e.Logger.SetLevel(log.INFO)

	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			if !ok {
				err = fmt.Errorf("pkg: %v", r)
			}
			errRecovered = err
			e.Logger.Error(err)
		}
	}()

	buildInfoMessage := fmt.Sprintf("BuildVersion: %s, BuildTime: %s, BuildCommit: %s",
		buildversion.BuildVersion, buildversion.BuildTime, buildversion.BuildCommit)
	e.Logger.Infof(buildInfoMessage)
	fmt.Println(buildInfoMessage)

	err := godotenv.Load(".env")
	if err != nil {
		e.Logger.Error(err)
	}

	webflowToken = os.Getenv("WEBFLOW_TOKEN")
	webflowCollection = os.Getenv("WEBFLOW_COLLECTION_ID")

	host, port, dbname, _, err := openDB()
	if err != nil {
		e.Logger.Error(err)
		panic(fmt.Errorf("failed to load database driver. host: %s, port: %d, dbname: %s, err: %+v", host, port, dbname, err))
	}
	defer func() {
		_ = db.Close()
	}()

	err = db.Ping()
	if err != nil {
		e.Logger.Error(err)
		panic(fmt.Errorf("failed to ping database. host: %s, port: %d, dbname: %s, err: %+v", host, port, dbname, err))
	}

	err = migrateDB(db)
	if err != nil {
		e.Logger.Error(err)
		panic(fmt.Errorf("failed to migrate database. err: %+v", err))
	}

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, fmt.Sprintf("I am ALIVE. %s", buildInfoMessage))
	})

	// Source Control Provider endpoints
	scpGroup := e.Group(SourceControlProvider)
	scpGroup.GET(
		fmt.Sprintf("%s", List),
		getSourceControlProviders).Name = "scp-list"

	// Organization related endpoints
	organizationGroup := e.Group(Organization)

	organizationGroup.GET(
		fmt.Sprintf("%s", List),
		getOrganizations).Name = "organization-list"
	organizationGroup.PUT(
		fmt.Sprintf("%s", Add),
		addOrganization).Name = "organization-add"
	organizationGroup.DELETE(
		fmt.Sprintf("%s/:%s/:%s", Delete, ParamScpName, ParamOrganizationName),
		deleteOrganization).Name = "organization-delete"

	// Participant related endpoints and group

	participantGroup := e.Group(Participant)

	participantGroup.GET(
		fmt.Sprintf("%s/:%s/:%s/:%s", Detail, ParamCampaignName, ParamScpName, ParamLoginName),
		getParticipantDetail).Name = "participant-detail"

	participantGroup.GET(
		fmt.Sprintf("%s/:%s", List, ParamCampaignName),
		getParticipantsList).Name = "participant-list"

	participantGroup.POST(Update, updateParticipant).Name = "participant-update"
	participantGroup.PUT(Add, logAddParticipant).Name = "participant-add"
	participantGroup.DELETE(
		fmt.Sprintf("%s/:%s/:%s", Delete, ParamScpName, ParamLoginName),
		deleteParticipant,
	)

	// Team related endpoints and group

	teamGroup := e.Group(Team)

	teamGroup.PUT(Add, addTeam)
	teamGroup.PUT(fmt.Sprintf("%s/:%s/:%s/:%s/:%s", Person, ParamCampaignName, ParamScpName, ParamLoginName, ParamTeamName), addPersonToTeam)

	// Bug related endpoints and group

	bugGroup := e.Group(Bug)

	bugGroup.PUT(Add, addBug)
	bugGroup.POST(fmt.Sprintf("%s/:%s/:%s/:%s", Update, ParamCampaignName, ParamBugCategory, ParamPointValue), updateBug)
	bugGroup.GET(List, getBugs)
	bugGroup.PUT(List, putBugs)

	// Campaign related endpoints and group

	campaignGroup := e.Group(Campaign)

	campaignGroup.GET(fmt.Sprintf("%s", List), getCampaigns)
	campaignGroup.GET(fmt.Sprintf("%s", "/current"), getCurrentCampaignEcho)
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

func openDB() (host string, port int, dbname, sslMode string, err error) {
	host = os.Getenv(envPGHost)
	port, _ = strconv.Atoi(os.Getenv(envPGPort))
	user := os.Getenv(envPGUsername)
	password := os.Getenv(envPGPassword)
	dbname = os.Getenv(envPGDBName)
	sslMode = os.Getenv(envSSLMode)

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslMode)
	db, err = sql.Open("postgres", psqlInfo)
	return
}

type ParticipantCreateError struct {
	Status string
}

func (e *ParticipantCreateError) Error() string {
	return fmt.Sprintf("could not create upstream participant. response status: %s", e.Status)
}

func upstreamNewParticipant(c echo.Context, p participant) (id string, err error) {
	item := leaderboardItem{}
	item.UserName = p.LoginName
	item.Slug = p.LoginName
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

	c.Logger().Debugf("created new upstream user: leaderboardItem: %+v", item)
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

const sqlSelectSourceControlProvider = `SELECT * FROM source_control_provider`

func getSourceControlProviders(c echo.Context) (err error) {
	rows, err := db.Query(sqlSelectSourceControlProvider)
	if err != nil {
		return
	}

	var scps []sourceControlProvider
	for rows.Next() {
		scp := sourceControlProvider{}
		err = rows.Scan(&scp.ID, &scp.SCPName, &scp.Url)
		if err != nil {
			return
		}
		scps = append(scps, scp)
	}

	return c.JSON(http.StatusOK, scps)
}

const sqlAddOrganization = `INSERT INTO organization
		(fk_scp, organization)
		VALUES ((SELECT id FROM source_control_provider WHERE name = $1), $2)
		RETURNING Id`

func addOrganization(c echo.Context) (err error) {
	organization := organization{}

	err = json.NewDecoder(c.Request().Body).Decode(&organization)
	if err != nil {
		return
	}

	var guid string
	err = db.QueryRow(sqlAddOrganization, organization.SCPName, organization.Organization).
		Scan(&guid)
	if err != nil {
		c.Logger().Errorf("error inserting organization: %+v, err: %+v", organization, err)
		return
	}

	c.Logger().Debugf("added organization: %+v", organization)
	return c.String(http.StatusCreated, guid)
}

const sqlSelectOrganization = `SELECT
		organization.Id,
        Name,
        Organization
		FROM organization
		INNER JOIN source_control_provider ON fk_scp = source_control_provider.Id`

func getOrganizations(c echo.Context) (err error) {
	rows, err := db.Query(sqlSelectOrganization)
	if err != nil {
		return
	}

	var orgs []organization
	for rows.Next() {
		org := organization{}
		err = rows.Scan(&org.ID, &org.SCPName, &org.Organization)
		if err != nil {
			return
		}
		orgs = append(orgs, org)
	}

	return c.JSON(http.StatusOK, orgs)
}

const sqlDeleteOrganization = `DELETE FROM organization 
	WHERE fk_scp = (SELECT id from source_control_provider WHERE name = $1) 
	AND organization = $2`

func deleteOrganization(c echo.Context) (err error) {
	scpName := c.Param(ParamScpName)
	orgName := c.Param(ParamOrganizationName)
	res, err := db.Exec(sqlDeleteOrganization, scpName, orgName)
	if err != nil {
		return
	}
	rowsAffected, err := res.RowsAffected()
	c.Logger().Infof("delete organization: scpName: %s, name: %s, rowsAffected: %d", scpName, orgName, rowsAffected)
	if rowsAffected > 0 {
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusNotFound, fmt.Sprintf("no organization: scpName: %s, name: %s", scpName, orgName))
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

const sqlSelectOrganizationExists = `SELECT EXISTS(
		SELECT Id FROM organization
		WHERE fk_scp = (SELECT id from source_control_provider WHERE LOWER(name) = $1) AND Organization = $2)`

// check if repo is in participating set
func validOrganization(c echo.Context, msg scoringMessage) (orgExists bool) {
	row := db.QueryRow(sqlSelectOrganizationExists, msg.EventSource, msg.RepoOwner)
	err := row.Scan(&orgExists)
	if err != nil {
		c.Logger().Debugf("organization is not valid. scp: %s, organization: %s, err: %v", msg.EventSource, msg.RepoOwner, err)
		return
	}
	return
}

const sqlSelectParticipantId = `SELECT
		participant.Id,
       	source_control_provider.name
		FROM participant
		INNER JOIN campaign ON campaign.Id = fk_campaign
		INNER JOIN source_control_provider ON source_control_provider.Id = fk_scp
		WHERE campaign.name = $1 
		    AND LOWER(source_control_provider.name) = $2 
			AND login_name = $3`

func validScore(c echo.Context, msg scoringMessage) (isValidScore bool, campaign campaignStruct, scpName string) {
	// check if repo is in participating set
	if !validOrganization(c, msg) {
		c.Logger().Debugf("skip score-missing organization. owner: %s, user: %s", msg.RepoOwner, msg.TriggerUser)
		return
	}

	// find current campaign
	campaign, err := getCurrentCampaign()
	if err != nil {
		c.Logger().Errorf("error reading current campaign. msg: %+v, error: %+v", msg, err)
		return
	}

	// Check if participant is registered
	row := db.QueryRow(sqlSelectParticipantId, campaign.Name, msg.EventSource, msg.TriggerUser)
	var id string
	err = row.Scan(&id, &scpName) // this reads the db (capitalized) scpName
	if err != nil {
		c.Logger().Errorf("skip score-missing participant. msg: %+v, campaign: %+v, err: %v", msg, campaign, err)
		return
	}
	isValidScore = true
	return
}

const sqlSelectPointValue = `SELECT pointValue, campaign.name FROM bug 
	INNER JOIN campaign ON campaign.Id = bug.fk_campaign	
	WHERE category = $1`

func scorePoints(c echo.Context, msg scoringMessage) (points int, campaignName string) {
	points = 0
	scored := 0

	for bugType, count := range msg.BugCounts {
		row := db.QueryRow(sqlSelectPointValue, bugType)
		var value = 1
		if err := row.Scan(&value, &campaignName); err != nil {
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

const sqlUpdateParticipantScore = `UPDATE participant 
		SET Score = Score + $1 
		WHERE fk_campaign = (SELECT id FROM campaign WHERE name = $2)
		      AND fk_scp = (SELECT id FROM source_control_provider WHERE name = $3)
		      AND login_name = $4 
		RETURNING UpstreamId, Score`

func updateParticipantScore(c echo.Context, campaign, scpName, loginName string, delta int) (err error) {
	var id string
	var score int
	row := db.QueryRow(sqlUpdateParticipantScore, delta, campaign, scpName, loginName)
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
			FROM scoring_event
			WHERE fk_campaign = (SELECT id FROM campaign WHERE name = $1)
			    AND fk_scp = (SELECT id FROM source_control_provider WHERE name = $2)
			    AND repoOwner = $3
				AND repoName = $4
				AND pr = $5`

const sqlInsertScoringEvent = `INSERT INTO scoring_event
			(fk_campaign, fk_scp, repoOwner, repoName, pr, username, points)
			VALUES ((SELECT id FROM campaign WHERE name = $1), 
			        (SELECT id FROM source_control_provider WHERE name = $2),
			        $3, $4, $5, $6, $7)
			ON CONFLICT (fk_campaign, fk_scp, repoOwner, repoName, pr) DO
				UPDATE SET points = $7`

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
		// force triggerUser to lower case to match database/webflow values
		msg.TriggerUser = strings.ToLower(msg.TriggerUser)

		// if this particular entry is not valid, ignore it and continue processing
		isValidScore, campaign, scpName := validScore(c, msg)
		if !isValidScore {
			continue
		}
		if campaign.Name == "" {
			err = fmt.Errorf("empty current campaign name. campaign: %+v", campaign)
			return
		}
		campaignName := campaign.Name
		if scpName == "" {
			err = fmt.Errorf("empty db scpName. campaign: %+v", campaign)
			return
		}

		//newPoints, campaignName := scorePoints(c, msg)
		newPoints, _ := scorePoints(c, msg)

		var tx *sql.Tx
		tx, err = db.Begin()
		if err != nil {
			return
		}

		row := db.QueryRow(sqlScoreQuery, campaignName, scpName, msg.RepoOwner, msg.RepoName, msg.PullRequest)
		oldPoints := 0
		err = row.Scan(&oldPoints)
		if err != nil {
			// ignore error case from scan when no row exists, will occur when this is a new score event
			c.Logger().Debugf("ignoring likely new score event. err: %+v, scoringMessage: %+v", err, msg)
		}

		_, err = db.Exec(sqlInsertScoringEvent, campaignName, scpName, msg.RepoOwner, msg.RepoName, msg.PullRequest, msg.TriggerUser, newPoints)
		if err != nil {
			return
		}

		err = updateParticipantScore(c, campaignName, scpName, msg.TriggerUser, newPoints-oldPoints)
		if err != nil {
			return
		}

		err = tx.Commit()
		if err != nil {
			return
		}

		c.Logger().Debugf("score updated. newPoints: %d, oldPoints: %d, scoringMessage: %+v", newPoints, oldPoints, msg)
	}

	//c.Logger().Debugf("scoringAlert completed: %+v", alert)

	return c.NoContent(http.StatusAccepted)
}

const sqlSelectParticipantDetail = `SELECT 
		participant.Id, campaign.name, source_control_provider.name, login_name, Email, DisplayName, Score, team.TeamName, JoinedAt
		FROM participant
		LEFT JOIN team ON team.Id = participant.fk_team
		INNER JOIN campaign ON campaign.Id = participant.fk_campaign
		INNER JOIN source_control_provider ON participant.fk_scp = source_control_provider.Id
		WHERE campaign.name = $1
		  AND source_control_provider.name = $2 
		  AND participant.login_name = $3`

func getParticipantDetail(c echo.Context) (err error) {
	campaignName := c.Param(ParamCampaignName)
	scpName := c.Param(ParamScpName)
	loginName := c.Param(ParamLoginName)
	c.Logger().Debugf("Getting detail for campaignName: %s, scpName: %s, loginName: %s", campaignName, scpName, loginName)

	row := db.QueryRow(sqlSelectParticipantDetail, campaignName, scpName, loginName)

	participant := new(participant)
	err = row.Scan(&participant.ID,
		&participant.CampaignName,
		&participant.ScpName,
		&participant.LoginName,
		&participant.Email,
		&participant.DisplayName,
		&participant.Score,
		&participant.fkTeam,
		&participant.JoinedAt,
	)

	if err != nil {
		c.Logger().Error(err)
		return
	}

	return c.JSON(http.StatusOK, participant)
}

const sqlSelectParticipantsByCampaign = `SELECT
		participant.Id, campaign.name, source_control_provider.name, login_name, Email, DisplayName, Score, team.TeamName, JoinedAt 
		FROM participant
		LEFT JOIN team ON participant.fk_team = team.Id
		INNER JOIN campaign ON participant.fk_campaign = campaign.Id
		INNER JOIN source_control_provider ON participant.fk_scp = source_control_provider.Id
		WHERE campaign.name = $1`

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
			&participant.CampaignName,
			&participant.ScpName,
			&participant.LoginName,
			&participant.Email,
			&participant.DisplayName,
			&participant.Score,
			&participant.fkTeam,
			&participant.JoinedAt,
		)
		if err != nil {
			return
		}
		participants = append(participants, *participant)
	}

	return c.JSON(http.StatusOK, participants)
}

const sqlUpdateParticipant = `UPDATE participant 
		SET 
		    fk_campaign = (SELECT Id FROM campaign WHERE name = $1),
		    fk_scp = (SELECT Id FROM source_control_provider WHERE name = $2),
		    login_name = $3,
		    Email = $4,
		    DisplayName = $5,
		    Score = $6,
		    fk_team = $7		    
		WHERE Id = $8`

func updateParticipant(c echo.Context) (err error) {
	participant := participant{}

	err = json.NewDecoder(c.Request().Body).Decode(&participant)
	if err != nil {
		return
	}

	res, err := db.Exec(
		sqlUpdateParticipant,
		participant.CampaignName,
		participant.ScpName,
		participant.LoginName,
		participant.Email,
		participant.DisplayName,
		participant.Score,
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

	err = updateParticipantScore(c, participant.CampaignName, participant.ScpName, participant.LoginName, 0)
	if err != nil {
		return
	}

	if rows == 1 {
		c.Logger().Infof(
			"Success, huzzah! Participant updated, ID: %s, loginName: %s",
			participant.ID,
			participant.LoginName,
		)

		return c.NoContent(http.StatusNoContent)
	} else {
		c.Logger().Errorf(
			"No row was updated, something goofy has occurred, ID: %s, loginName: %s, rows: %s",
			participant.ID,
			participant.LoginName,
			rows,
		)

		return c.NoContent(http.StatusBadRequest)
	}
}

const sqlDeleteParticipant = `DELETE FROM participant WHERE 
                              fk_scp = (SELECT id from source_control_provider where name =$1)
                          AND login_name = $2`

func deleteParticipant(c echo.Context) (err error) {
	scpName := c.Param(ParamScpName)
	loginName := c.Param(ParamLoginName)
	result, err := db.Exec(sqlDeleteParticipant, scpName, loginName)
	if err != nil {
		c.Logger().Debugf("error deleting participant: scpName: %s, loginName %s, err: %+v", scpName, loginName, err)
		return
	}
	rowsAffected, err := result.RowsAffected()
	// ignore any error retrieving rows affected for now
	c.Logger().Debugf("delete participant: scpName: %s, loginName %s, rows affected: %d, err: %+v", scpName, loginName, err)
	if rowsAffected > 0 {
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusNotFound, fmt.Sprintf("no participant: scpName: %s, loginName: %s", scpName, loginName))
}

const sqlInsertParticipant = `INSERT INTO participant 
		(fk_scp, fk_campaign, login_name, Email, DisplayName, Score, UpstreamId) 
		VALUES ((SELECT Id FROM source_control_provider WHERE Name = $1),
		        (SELECT Id FROM campaign WHERE name = $2),
		        $3, $4, $5, $6, $7)
		RETURNING Id, Score, JoinedAt`

// was not seeing enough detail when addParticipant() returns error, so capturing such cases in the log.
func logAddParticipant(c echo.Context) (err error) {
	if err = addParticipant(c); err != nil {
		c.Logger().Errorf("error calling addParticipant. err: %+v", err)
	}
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

	var guid string
	err = db.QueryRow(
		sqlInsertParticipant,
		participant.ScpName,
		participant.CampaignName,
		participant.LoginName,
		participant.Email,
		participant.DisplayName,
		0,
		webflowId).Scan(&guid, &participant.Score, &participant.JoinedAt)
	if err != nil {
		c.Logger().Errorf("error inserting participant: %+v, err: %+v", participant, err)
		return
	}

	participant.ID = guid

	detailUri := c.Echo().Reverse("participant-detail", participant.LoginName)
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

const sqlInsertTeam = `INSERT INTO team
		(fk_campaign, TeamName)
		VALUES ((SELECT id FROM campaign WHERE name = $1), $2)
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
		team.Campaign,
		team.TeamName).Scan(&guid)
	if err != nil {
		return
	}

	return c.String(http.StatusCreated, guid)
}

const sqlUpdateParticipantTeam = `UPDATE participant 
		SET fk_team = (SELECT Id FROM team WHERE TeamName = $1)
		WHERE fk_campaign = (SELECT id FROM campaign WHERE name = $2)
		 AND fk_scp = (SELECT id FROM source_control_provider WHERE name = $3)
		 AND login_name = $4`

func addPersonToTeam(c echo.Context) (err error) {
	teamName := c.Param(ParamTeamName)
	campaignName := c.Param(ParamCampaignName)
	scpName := c.Param(ParamScpName)
	loginName := c.Param(ParamLoginName)

	if teamName == "" || campaignName == "" || scpName == "" || loginName == "" {
		return c.NoContent(http.StatusBadRequest)
	}

	res, err := db.Exec(
		sqlUpdateParticipantTeam,
		teamName,
		campaignName,
		scpName,
		loginName)
	if err != nil {
		return
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return
	}

	if rows > 0 {
		c.Logger().Infof(
			"Success, huzzah! Row updated, teamName: %s, campaignName: %s, scpName: %s, login_name: %s",
			teamName, campaignName, scpName, loginName,
		)

		return c.NoContent(http.StatusNoContent)
	} else {
		c.Logger().Errorf(
			"No row was updated, something goofy has occurred, teamName: %s, campaignName: %s, scpName: %s, login_name: %s",
			teamName, campaignName, scpName, loginName,
		)

		return c.NoContent(http.StatusBadRequest)
	}
}

func validateBug(c echo.Context, bugToValidate bug) (err error) {
	if len(bugToValidate.Campaign) == 0 {
		err = fmt.Errorf("bug is not valid, empty campaign: bug: %+v", bugToValidate)
	} else if len(bugToValidate.Category) == 0 {
		err = fmt.Errorf("bug is not valid, empty category: bug: %+v", bugToValidate)
	} else if bugToValidate.PointValue < 0 {
		err = fmt.Errorf("bug is not valid, negative PointValue: bug: %+v", bugToValidate)
	}
	if err != nil {
		c.Logger().Error(err)
	}
	return
}

const sqlInsertBug = `INSERT INTO bug
		(fk_campaign, category, pointValue)
		VALUES ((SELECT id FROM campaign WHERE name = $1), $2, $3)
		RETURNING ID`

func addBug(c echo.Context) (err error) {
	bug := bug{}

	err = json.NewDecoder(c.Request().Body).Decode(&bug)
	if err != nil {
		c.Logger().Errorf("error decoding bug. body: err: %+v", err)
		return
	}

	if err = validateBug(c, bug); err != nil {
		return
	}

	var guid string
	err = db.QueryRow(sqlInsertBug, bug.Campaign, bug.Category, bug.PointValue).Scan(&guid)
	if err != nil {
		c.Logger().Errorf("error inserting bug: %+v, err: %+v", bug, err)
		return
	}
	bug.Id = guid
	creation := creationResponse{
		Id:     guid,
		Object: bug,
	}
	return c.JSON(http.StatusCreated, creation)
}

const sqlUpdateBug = `UPDATE bug
		SET pointValue = $1
		WHERE fk_campaign = (SELECT id FROM campaign WHERE name = $2) AND category = $3`

func updateBug(c echo.Context) (err error) {
	campaign := c.Param(ParamCampaignName)
	category := c.Param(ParamBugCategory)
	pointValue, err := strconv.Atoi(c.Param(ParamPointValue))
	if err != nil {
		return
	}

	if err = validateBug(c, bug{Campaign: campaign, Category: category, PointValue: pointValue}); err != nil {
		return
	}

	c.Logger().Debug(category)

	res, err := db.Exec(sqlUpdateBug, pointValue, campaign, category)
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

const sqlSelectBug = `SELECT bug.id, campaign.name, category, pointvalue FROM bug
		INNER JOIN campaign ON fk_campaign = campaign.Id`

func getBugs(c echo.Context) (err error) {

	rows, err := db.Query(sqlSelectBug)
	if err != nil {
		return
	}

	var bugs []bug
	for rows.Next() {
		bug := bug{}
		err = rows.Scan(&bug.Id, &bug.Campaign, &bug.Category, &bug.PointValue)
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
		c.Logger().Errorf("error decoding bug. body: err: %+v", err)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		return
	}
	var inserted []bug
	for _, bug := range bugs {
		if err = validateBug(c, bug); err != nil {
			return
		}

		err = db.QueryRow(sqlInsertBug, bug.Campaign, bug.Category, bug.PointValue).Scan(&bug.Id)
		if err != nil {
			c.Logger().Errorf("error inserting bug: %+v, err: %+v", bug, err)
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

const sqlSelectCampaign = `SELECT * FROM campaign`

func getCampaigns(c echo.Context) (err error) {
	rows, err := db.Query(
		sqlSelectCampaign)
	if err != nil {
		return
	}

	var campaigns []campaignStruct
	for rows.Next() {
		campaign := campaignStruct{}
		err = rows.Scan(&campaign.ID, &campaign.Name, &campaign.CreatedOn, &campaign.CreatedOrder, &campaign.Active, &campaign.Note)
		if err != nil {
			return
		}
		campaigns = append(campaigns, campaign)
	}

	return c.JSON(http.StatusOK, campaigns)
}

const sqlSelectCurrentCampaign = `SELECT * FROM campaign
		WHERE campaign.active = true
		ORDER BY campaign.create_order DESC`

func getCurrentCampaign() (current campaignStruct, err error) {
	err = db.QueryRow(
		sqlSelectCurrentCampaign).Scan(&current.ID, &current.Name, &current.CreatedOn, &current.CreatedOrder, &current.Active, &current.Note)
	if err != nil {
		return
	}

	return
}

func getCurrentCampaignEcho(c echo.Context) (err error) {
	current, err := getCurrentCampaign()
	if err != nil {
		return c.String(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, current)
}

const sqlInsertCampaign = `INSERT INTO campaign 
		(name) 
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

func migratePrep(db *sql.DB) (m *migrate.Migrate, err error) {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return
	}

	m, err = migrate.NewWithDatabaseInstance(
		"file://db/migrations/v2",
		"postgres", driver)
	if migrateErrorApplicable(err) {
		return
	}
	return
}

func downgradeDB(db *sql.DB) (err error) {
	// Don't run this, will delete db stuff, for use in testing only
	m, err := migratePrep(db)
	if err != nil {
		return
	}

	if err = m.Down(); err != nil {
		if migrateErrorApplicable(err) {
			return
		} else {
			err = nil
		}
	}

	return
}

func migrateDB(db *sql.DB) (err error) {
	m, err := migratePrep(db)
	if err != nil {
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
