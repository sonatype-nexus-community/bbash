package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

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

	e.GET("/", handleRoot)

	participantGroup := e.Group(PARTICIPANT)

	participantGroup.GET(DETAIL, getParticipantDetail)
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
	return
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
