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
	Score        string `json:"score"`
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
	TEAM                string = "/team"
	ADD                 string = "/add"
	PERSON              string = "/person"
	BUG                 string = "/bug"
	CAMPAIGN            string = "/campaign"
)

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

	routes := e.Routes()

	for _, v := range routes {
		fmt.Printf("Registered route: %s %s as %s\n", v.Method, v.Path, v.Name)
	}

	e.Start(":7777")
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
		WHERE Id = $7
		RETURNING Id`

	var guid string
	err = db.QueryRow(
		sqlUpdate,
		participant.GitHubName,
		participant.Email,
		participant.DisplayName,
		participant.Score,
		participant.CampaignName,
		participant.fkTeam,
		participant.ID,
	).Scan(&guid)
	if err != nil {
		return
	}

	participant.ID = guid

	return c.NoContent(http.StatusNoContent)
}

func addParticipant(c echo.Context) (err error) {
	participant := participant{}

	err = json.NewDecoder(c.Request().Body).Decode(&participant)
	if err != nil {
		return
	}

	sqlInsert := `INSERT INTO participants 
		(GithubName, Email, DisplayName, Score, Campaign) 
		VALUES ($1, $2, $3, $4, (SELECT Id FROM campaigns WHERE CampaignName = $5))
		RETURNING Id, Score, JoinedAt`

	var guid string
	err = db.QueryRow(
		sqlInsert,
		participant.GitHubName,
		participant.Email,
		participant.DisplayName,
		0,
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
