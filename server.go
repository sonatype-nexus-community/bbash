package main

import (
	"database/sql"
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
	ID          string         `json:"guid"`
	GitHubName  string         `json:"gitHubName"`
	email       string         `json:"email"`
	DisplayName string         `json:"displayName"`
	Score       string         `json:"score"`
	fkTeam      sql.NullString `json:"team"`
	JoinedAt    time.Time      `json:"joinedAt"`
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

	participantGroup := e.Group(PARTICIPANT)

	participantGroup.GET(
		fmt.Sprintf("%s/:id", DETAIL),
		getParticipantDetail)

	participantGroup.GET(LIST, getParticipantsList)
	participantGroup.POST(UPDATE, updateParticipant)
	participantGroup.PUT(ADD, addParticipant)

	teamGroup := e.Group(TEAM)

	teamGroup.PUT(ADD, addTeam)
	teamGroup.PUT(PERSON, addPersonToTeam)

	bugGroup := e.Group(BUG)

	bugGroup.PUT(ADD, addBug)
	bugGroup.POST(UPDATE, updateBug)

	bugsGroup := e.Group(BUGS)

	bugsGroup.GET(LIST, getBugs)
	bugsGroup.PUT(LIST, putBugs)

	e.Logger.Fatal(e.Start(":7777"))
}

func getParticipantDetail(c echo.Context) (err error) {
	gitHubName := c.Param("id")
	c.Logger().Debug("Getting detail for ", gitHubName)

	sqlQuery := `SELECT * FROM participants WHERE GitHubName = $1`

	row := db.QueryRow(sqlQuery, gitHubName)

	participant := new(participant)
	err = row.Scan(&participant.ID,
		&participant.GitHubName,
		&participant.email,
		&participant.DisplayName,
		&participant.Score,
		&participant.fkTeam,
		&participant.JoinedAt)

	if err != nil {
		return
	}

	return c.JSON(http.StatusOK, participant)
}

func getParticipantsList(c echo.Context) (err error) {
	return
}

func updateParticipant(c echo.Context) (err error) {
	return
}

func addParticipant(c echo.Context) (err error) {
	return
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

func handleRoot(c echo.Context) (err error) {
	return c.String(http.StatusOK, "I'm Fine")
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
