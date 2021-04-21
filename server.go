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
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
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

func main() {

	const (
		PARTICIPANT string = "/participant"
		DETAIL      string = "/detail"
		LIST        string = "/list"
		UPDATE      string = "/update"
		TEAM        string = "/team"
		ADD         string = "/add"
		PERSON      string = "/person"
		BUG         string = "/bug"
		BUGS        string = "/bugs"
	)

	e := echo.New()
	e.Debug = true

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
	defer db.Close()

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
		fmt.Sprintf("%s/:id", DETAIL),
		getParticipantDetail)

	participantGroup.GET(
		fmt.Sprintf("%s/:campaign", LIST),
		getParticipantsList)

	participantGroup.POST(UPDATE, updateParticipant)
	participantGroup.PUT(ADD, addParticipant)

	// Team related endpoints and group

	teamGroup := e.Group(TEAM)

	teamGroup.PUT(ADD, addTeam)
	teamGroup.PUT(PERSON, addPersonToTeam)

	// Bug related endpoints and group

	bugGroup := e.Group(BUG)

	bugGroup.PUT(ADD, addBug)
	bugGroup.POST(UPDATE, updateBug)

	// Bugs related endpoints and group

	bugsGroup := e.Group(BUGS)

	bugsGroup.GET(LIST, getBugs)
	bugsGroup.PUT(LIST, putBugs)

	e.Logger.Fatal(e.Start(":7777"))
}

func getParticipantDetail(c echo.Context) (err error) {
	gitHubName := c.Param("id")
	c.Logger().Debug("Getting detail for ", gitHubName)

	sqlQuery := `SELECT * FROM participants 
		WHERE GitHubName = $1`

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
		return
	}

	return c.JSON(http.StatusOK, participant)
}

func getParticipantsList(c echo.Context) (err error) {
	campaignName := c.Param("campaign")
	c.Logger().Debug("Getting list for ", campaignName)

	sqlQuery := `SELECT * FROM participants 
		WHERE CampaignName = $1`

	rows, err := db.Query(sqlQuery, campaignName)
	if err != nil {
		return
	}

	participants := []participant{}
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
	return
}

func addParticipant(c echo.Context) (err error) {
	participant := participant{}

	err = json.NewDecoder(c.Request().Body).Decode(&participant)
	if err != nil {
		return
	}

	sqlInsert := `INSERT INTO participants 
		(GithubName, Email, DisplayName, Score) 
		VALUES ($1, $2, $3, $4, $5)`

	_, err = db.Exec(sqlInsert, participant.GitHubName, participant.Email, participant.DisplayName, 0, participant.CampaignName)
	if err != nil {
		return
	}

	return c.NoContent(http.StatusOK)
}

func addTeam(c echo.Context) (err error) {
	return
}

func addPersonToTeam(c echo.Context) (err error) {
	return
}

func addBug(c echo.Context) (err error) {
	return
}

func updateBug(c echo.Context) (err error) {
	return
}

func getBugs(c echo.Context) (err error) {
	return
}

func putBugs(c echo.Context) (err error) {
	return
}

func migrateDB(db *sql.DB) (err error) {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://db/migrations",
		"postgres", driver)

	if err != nil {
		return
	}

	if err = m.Up(); err != nil {
		return
	}

	return
}
