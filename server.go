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

package main

import (
	"crypto/subtle"
	"database/sql"

	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4/middleware"
	"github.com/sonatype-nexus-community/bbash/internal/db"
	"github.com/sonatype-nexus-community/bbash/internal/types"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sonatype-nexus-community/bbash/buildversion"

	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
)

var postgresDB db.IBBashDB

type creationResponse struct {
	Id        string                 `json:"guid"`
	Endpoints map[string]interface{} `json:"endpoints"`
	Object    interface{}            `json:"object"`
}

type endpointDetail struct {
	URI  string `json:"uri"`
	Verb string `json:"httpVerb"`
}

type scoringAlert struct {
	RecentHits []string `json:"recent_hits"` // encoded scoring message
}

const (
	ParamScpName          string = "scpName"
	ParamLoginName        string = "loginName"
	ParamCampaignName     string = "campaignName"
	ParamTeamName         string = "teamName"
	ParamBugCategory      string = "bugCategory"
	ParamPointValue       string = "pointValue"
	ParamOrganizationName string = "organizationName"
	pathAdmin             string = "/admin"
	SourceControlProvider string = "/scp"
	Organization          string = "/organization"
	Participant           string = "/participant"
	Detail                string = "/detail"
	List                  string = "/list"
	active                string = "/active"
	Update                string = "/update"
	Delete                string = "/delete"
	Team                  string = "/team"
	Add                   string = "/add"
	Person                string = "/person"
	Bug                   string = "/bug"
	Campaign              string = "/campaign"
	ScoreEvent            string = "/scoring"
	New                   string = "/new"
	buildLocation         string = "build"
)

const defaultServicePort = ":7777"

const envPGHost = "PG_HOST"
const envPGPort = "PG_PORT"
const envPGUsername = "PG_USERNAME"
const envPGPassword = "PG_PASSWORD"
const envPGDBName = "PG_DB_NAME"
const envSSLMode = "SSL_MODE"
const envAdminUsername = "ADMIN_USERNAME"
const envAdminPassword = "ADMIN_PASSWORD"

var errRecovered error
var logger *zap.Logger

func main() {
	e := echo.New()

	var err error
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	logger, err = config.Build()
	if err != nil {
		e.Logger.Fatal("can not initialize zap logger: %+v", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	// NOTE: using middleware.Logger() makes lots of AWS ELB Healthcheck noise in server logs
	//e.Use(middleware.Logger(), /* Log everything to stdout*/)
	//e.Use(echozap.ZapLogger(logger))
	e.Use(ZapLoggerFilterAwsElb(logger))

	e.Debug = true

	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			if !ok {
				err = fmt.Errorf("pkg: %v", r)
			}
			errRecovered = err
			logger.Error("panic", zap.Error(err))
		}
	}()

	buildInfoMessage := fmt.Sprintf("BuildVersion: %s, BuildTime: %s, BuildCommit: %s",
		buildversion.BuildVersion, buildversion.BuildTime, buildversion.BuildCommit)
	logger.Info("build", zap.String("buildMsg", buildInfoMessage))
	fmt.Println(buildInfoMessage)

	err = godotenv.Load(".env")
	if err != nil {
		logger.Error("env load", zap.Error(err))
	}

	pg, host, port, dbname, _, err := openDB()
	if err != nil {
		logger.Error("db open", zap.Error(err))
		panic(fmt.Errorf("failed to load database driver. host: %s, port: %d, dbname: %s, err: %+v", host, port, dbname, err))
	}
	defer func() {
		if err := pg.Close(); err != nil {
			logger.Error("db close", zap.Error(err))
		}
	}()

	err = pg.Ping()
	if err != nil {
		logger.Error("db ping", zap.Error(err))
		panic(fmt.Errorf("failed to ping database. host: %s, port: %d, dbname: %s, err: %+v", host, port, dbname, err))
	}

	postgresDB = db.New(pg, logger)

	err = postgresDB.MigrateDB("file://internal/db/migrations/v2")
	if err != nil {
		logger.Error("db migrate", zap.Error(err))
		panic(fmt.Errorf("failed to migrate database. err: %+v", err))
	} else {
		logger.Info("db migration complete")
	}

	setupRoutes(e, buildInfoMessage)

	logger.Fatal("application end", zap.Error(e.Start(defaultServicePort)))
}

func setupRoutes(e *echo.Echo, buildInfoMessage string) {
	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, fmt.Sprintf("I am ALIVE. %s", buildInfoMessage))
	})

	// admin endpoint group
	adminGroup := e.Group(pathAdmin, middleware.BasicAuth(infoBasicValidator))

	// Source Control Provider endpoints
	scpGroup := adminGroup.Group(SourceControlProvider)
	scpGroup.GET(List, getSourceControlProviders).Name = "scp-list"

	// Organization related endpoints
	organizationGroup := adminGroup.Group(Organization)

	organizationGroup.GET(List, getOrganizations).Name = "OrganizationStruct-list"
	organizationGroup.PUT(Add, addOrganization).Name = "OrganizationStruct-add"
	organizationGroup.DELETE(
		fmt.Sprintf("%s/:%s/:%s", Delete, ParamScpName, ParamOrganizationName),
		deleteOrganization).Name = "OrganizationStruct-delete"

	// Participant related endpoints and group

	publicParticipantGroup := e.Group(Participant)
	publicParticipantGroup.GET(
		fmt.Sprintf("%s/:%s", List, ParamCampaignName),
		getParticipantsList).Name = "participant-list"

	participantGroup := adminGroup.Group(Participant)
	participantGroup.GET(
		fmt.Sprintf("%s/:%s/:%s/:%s", Detail, ParamCampaignName, ParamScpName, ParamLoginName),
		getParticipantDetail).Name = "participant-detail"

	participantGroup.POST(Update, updateParticipant).Name = "participant-update"
	participantGroup.PUT(Add, logAddParticipant).Name = "participant-add"
	participantGroup.DELETE(
		fmt.Sprintf("%s/:%s/:%s/:%s", Delete, ParamCampaignName, ParamScpName, ParamLoginName),
		deleteParticipant,
	)

	// Team related endpoints and group

	teamGroup := adminGroup.Group(Team)

	teamGroup.PUT(Add, addTeam)
	teamGroup.PUT(fmt.Sprintf("%s/:%s/:%s/:%s/:%s", Person, ParamCampaignName, ParamScpName, ParamLoginName, ParamTeamName), addPersonToTeam)

	// Bug related endpoints and group

	bugGroup := adminGroup.Group(Bug)

	bugGroup.PUT(Add, addBug)
	bugGroup.POST(fmt.Sprintf("%s/:%s/:%s/:%s", Update, ParamCampaignName, ParamBugCategory, ParamPointValue), updateBug)
	bugGroup.GET(List, getBugs)
	bugGroup.PUT(List, putBugs)

	// Campaign related endpoints and group

	publicCampaignGroup := e.Group(Campaign)
	publicCampaignGroup.GET(active, getActiveCampaigns)

	campaignGroup := adminGroup.Group(Campaign)
	campaignGroup.GET(List, getCampaigns)
	campaignGroup.PUT(fmt.Sprintf("%s/:%s", Add, ParamCampaignName), addCampaign)
	campaignGroup.PUT(fmt.Sprintf("%s/:%s", Update, ParamCampaignName), updateCampaign)

	// Scoring related endpoints and group
	// @TODO put this endpoint behind some auth, and update lift log scraper
	//scoreGroup := adminGroup.Group(ScoreEvent)
	scoreGroup := e.Group(ScoreEvent)
	scoreGroup.POST(New, logNewScore)

	e.Static("/", buildLocation)

	routes := e.Routes()

	for _, v := range routes {
		routeInfo := fmt.Sprintf("%s %s as %s", v.Method, v.Path, v.Name)
		logger.Info("route", zap.String("info", routeInfo))
	}
}

//goland:noinspection GoUnusedParameter
func infoBasicValidator(username, password string, c echo.Context) (isValidLogin bool, err error) {
	// Be careful to use constant time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(username), []byte(os.Getenv(envAdminUsername))) == 1 &&
		subtle.ConstantTimeCompare([]byte(password), []byte(os.Getenv(envAdminPassword))) == 1 {
		isValidLogin = true
	} else {
		logger.Info("failed info endpoint login",
			zap.String("username", username),
			zap.String("password", password),
		)
	}
	return
}

// ZapLoggerFilterAwsElb is a middleware and zap to provide an "access log" like logging for each request.
// Adapted from ZapLogger, until I find a better way to filter out AWS ELB Healthcheck messages.
func ZapLoggerFilterAwsElb(log *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			err := next(c)
			if err != nil {
				c.Error(err)
			}

			req := c.Request()
			res := c.Response()

			fields := []zapcore.Field{
				zap.String("remote_ip", c.RealIP()),
				zap.String("latency", time.Since(start).String()),
				zap.String("host", req.Host),
				zap.String("request", fmt.Sprintf("%s %s", req.Method, req.RequestURI)),
				zap.Int("status", res.Status),
				zap.Int64("size", res.Size),
				zap.String("user_agent", req.UserAgent()),
			}

			userAgent := req.UserAgent()
			if strings.Contains(userAgent, "ELB-HealthChecker") {
				//fmt.Printf("userAgent: %s\n", userAgent)
				// skip logging of this AWS ELB healthcheck
				return nil
			}

			id := req.Header.Get(echo.HeaderXRequestID)
			if id == "" {
				id = res.Header().Get(echo.HeaderXRequestID)
				fields = append(fields, zap.String("request_id", id))
			}

			n := res.Status
			switch {
			case n >= 500:
				log.With(zap.Error(err)).Error("Server error", fields...)
			case n >= 400:
				log.With(zap.Error(err)).Warn("Client error", fields...)
			case n >= 300:
				log.Info("Redirection", fields...)
			default:
				log.Info("Success", fields...)
			}

			return nil
		}
	}
}

func openDB() (db *sql.DB, host string, port int, dbname, sslMode string, err error) {
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

func getSourceControlProviders(c echo.Context) (err error) {
	var scps []types.SourceControlProviderStruct
	scps, err = postgresDB.GetSourceControlProviders()
	if err != nil {
		return
	}

	return c.JSON(http.StatusOK, scps)
}

func addOrganization(c echo.Context) (err error) {
	organization := types.OrganizationStruct{}

	err = json.NewDecoder(c.Request().Body).Decode(&organization)
	if err != nil {
		return
	}

	var guid string
	guid, err = postgresDB.InsertOrganization(&organization)
	if err != nil {
		logger.Error("error inserting OrganizationStruct", zap.Any("OrganizationStruct", organization), zap.Error(err))
		return
	}

	logger.Debug("added OrganizationStruct", zap.Any("OrganizationStruct", organization))
	return c.String(http.StatusCreated, guid)
}

func getOrganizations(c echo.Context) (err error) {
	var orgs []types.OrganizationStruct
	orgs, err = postgresDB.GetOrganizations()
	if err != nil {
		return
	}

	return c.JSON(http.StatusOK, orgs)
}

func deleteOrganization(c echo.Context) (err error) {
	scpName := c.Param(ParamScpName)
	orgName := c.Param(ParamOrganizationName)

	var rowsAffected int64
	rowsAffected, err = postgresDB.DeleteOrganization(scpName, orgName)
	if err != nil {
		return
	}
	logger.Info("delete OrganizationStruct",
		zap.String("scpName", scpName),
		zap.String("orgName", orgName),
		zap.Int64("rowsAffected", rowsAffected))
	if rowsAffected > 0 {
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusNotFound, fmt.Sprintf("no OrganizationStruct: scpName: %s, name: %s", scpName, orgName))
}

func validScore(msg *types.ScoringMessage, now time.Time) (participantsToScore []types.ParticipantStruct, err error) {
	// check if repo is in participating set
	isValidOrg, err := postgresDB.ValidOrganization(msg)
	if err != nil {
		logger.Debug("skip score-error reading OrganizationStruct", zap.Any("msg", msg), zap.Error(err))
		return
	}
	if !isValidOrg {
		logger.Debug("skip score-missing OrganizationStruct",
			zap.String("RepoOwner", msg.RepoOwner), zap.String("TriggerUser", msg.TriggerUser))
		return
	}

	// Check if participant is registered for an active campaign
	participantsToScore, err = postgresDB.SelectParticipantsToScore(msg, now)
	if err != nil {
		logger.Error("skip score-error reading participant", zap.Any("msg", msg), zap.Error(err))
		return
	}
	if len(participantsToScore) == 0 {
		logger.Debug("skip score-missing participant", zap.Any("msg", msg), zap.Error(err))
		return
	}
	return
}

func scorePoints(msg *types.ScoringMessage, campaignName string) (points int) {
	points = 0
	scored := 0

	for bugType, count := range msg.BugCounts {
		value := postgresDB.SelectPointValue(msg, campaignName, bugType)
		points += count * value
		scored += count
	}

	// add 1 point for all non-classified fixed bugs
	if scored < msg.TotalFixed {
		points += msg.TotalFixed - scored
	}

	return
}

// was not seeing enough detail when newScore() returns error, so capturing such cases in the log.
func logNewScore(c echo.Context) (err error) {
	if err = newScore(c); err != nil {
		logger.Error("error calling newScore", zap.Error(err))
	}
	return
}

func newScore(c echo.Context) (err error) {
	var alert scoringAlert
	err = json.NewDecoder(c.Request().Body).Decode(&alert)
	if err != nil {
		return
	}

	//logger.Debug("scoring event", zap.Any("scoringAlert", alert))

	now := time.Now()

	for _, rawMsg := range alert.RecentHits {
		var msg types.ScoringMessage
		err = json.Unmarshal([]byte(rawMsg), &msg)
		if err != nil {
			logger.Debug("error unmarshalling ScoringMessage",
				zap.Error(err), zap.String("rawMsg", rawMsg))
			return
		}
		// force triggerUser to lower case to match database values
		msg.TriggerUser = strings.ToLower(msg.TriggerUser)

		// if this particular entry is not valid, ignore it and continue processing
		var activeParticipantsToScore []types.ParticipantStruct
		activeParticipantsToScore, err = validScore(&msg, now)
		if err != nil {
			logger.Debug("error validating ScoringMessage", zap.Error(err), zap.Any("msg", msg))
			return
		}
		if len(activeParticipantsToScore) == 0 {
			continue
		}
		for i, participantToScore := range activeParticipantsToScore {

			newPoints := scorePoints(&msg, participantToScore.CampaignName)

			// fix warning: Implicit memory aliasing in for loop.
			//oldPoints := postgresDB.SelectPriorScore(&participantToScore, &msg)
			oldPoints := postgresDB.SelectPriorScore(&activeParticipantsToScore[i], &msg)

			err = postgresDB.InsertScoringEvent(&activeParticipantsToScore[i], &msg, newPoints)
			if err != nil {
				return
			}

			err = postgresDB.UpdateParticipantScore(&activeParticipantsToScore[i], newPoints-oldPoints)
			if err != nil {
				return
			}

			logger.Debug("score updated",
				zap.Int("newPoints", newPoints), zap.Int("oldPoints", oldPoints), zap.Any("ScoringMessage", msg))
		}
	}

	logger.Debug("scoringAlert completed", zap.Any("alert", alert))

	return c.NoContent(http.StatusAccepted)
}

func getParticipantDetail(c echo.Context) (err error) {
	campaignName := c.Param(ParamCampaignName)
	scpName := c.Param(ParamScpName)
	loginName := c.Param(ParamLoginName)
	logger.Debug("getting detail for campaign",
		zap.String("campaignName", campaignName), zap.String("scpName", scpName), zap.String("loginName", loginName))

	var participant *types.ParticipantStruct
	participant, err = postgresDB.SelectParticipantDetail(campaignName, scpName, loginName)
	if err != nil {
		return
	}

	return c.JSON(http.StatusOK, participant)
}

func getParticipantsList(c echo.Context) (err error) {
	campaignName := c.Param(ParamCampaignName)
	logger.Debug("Getting participant list for campaign", zap.String("campaignName", campaignName))

	var participants []types.ParticipantStruct
	participants, err = postgresDB.SelectParticipantsInCampaign(campaignName)
	if err != nil {
		return
	}

	return c.JSON(http.StatusOK, participants)
}

func updateParticipant(c echo.Context) (err error) {
	participant := types.ParticipantStruct{}

	err = json.NewDecoder(c.Request().Body).Decode(&participant)
	if err != nil {
		return
	}

	var rowsAffected int64
	rowsAffected, err = postgresDB.UpdateParticipant(&participant)
	if err != nil {
		return
	}

	if rowsAffected == 1 {
		logger.Info("participant updated", zap.Any("participant", participant))
		return c.NoContent(http.StatusNoContent)
	} else {
		logger.Error("no participant row was updated, something goofy has occurred",
			zap.Any("participant", participant), zap.Int64("rowsAffected", rowsAffected))
		return c.NoContent(http.StatusBadRequest)
	}
}

func deleteParticipant(c echo.Context) (err error) {
	campaign := c.Param(ParamCampaignName)
	scpName := c.Param(ParamScpName)
	loginName := c.Param(ParamLoginName)

	var participantId string
	participantId, err = postgresDB.DeleteParticipant(campaign, scpName, loginName)
	if err != nil {
		return
	}

	return c.JSON(http.StatusOK, fmt.Sprintf("deleted participant: campaign: %s, scpName: %s, loginName: %s, participant.id: %s",
		campaign, scpName, loginName, participantId))
}

// was not seeing enough detail when addParticipant() returns error, so capturing such cases in the log.
func logAddParticipant(c echo.Context) (err error) {
	if err = addParticipant(c); err != nil {
		logger.Error("error calling addParticipant", zap.Error(err))
	}
	return
}

func addParticipant(c echo.Context) (err error) {
	participant := types.ParticipantStruct{}

	err = json.NewDecoder(c.Request().Body).Decode(&participant)
	if err != nil {
		return
	}

	err = postgresDB.InsertParticipant(&participant)
	if err != nil {
		return
	}

	detailUri := c.Echo().Reverse("participant-detail", participant.LoginName)
	updateUri := c.Echo().Reverse("participant-update")
	endpoints := make(map[string]interface{})
	endpoints["participantDetail"] = endpointDetail{URI: detailUri, Verb: "GET"}
	endpoints["participantUpdate"] = endpointDetail{URI: updateUri, Verb: "PUT"}

	creation := creationResponse{
		Id:        participant.ID,
		Endpoints: endpoints,
		Object:    participant,
	}

	return c.JSON(http.StatusCreated, creation)
}

func addTeam(c echo.Context) (err error) {
	team := types.TeamStruct{}

	err = json.NewDecoder(c.Request().Body).Decode(&team)
	if err != nil {
		return
	}

	err = postgresDB.InsertTeam(&team)
	if err != nil {
		return
	}

	return c.String(http.StatusCreated, team.Id)
}

func addPersonToTeam(c echo.Context) (err error) {
	teamName := c.Param(ParamTeamName)
	campaignName := c.Param(ParamCampaignName)
	scpName := c.Param(ParamScpName)
	loginName := c.Param(ParamLoginName)

	if teamName == "" || campaignName == "" || scpName == "" || loginName == "" {
		return c.NoContent(http.StatusBadRequest)
	}

	var rowsAffected int64
	rowsAffected, err = postgresDB.UpdateParticipantTeam(teamName, campaignName, scpName, loginName)
	if err != nil {
		return
	}

	if rowsAffected > 0 {
		logger.Info("team updated",
			zap.String("teamName", teamName), zap.String("campaignName", campaignName),
			zap.String("scpName", scpName), zap.String("loginName", loginName))

		return c.NoContent(http.StatusNoContent)
	} else {
		logger.Error("no team row was updated, something goofy has occurred",
			zap.String("teamName", teamName), zap.String("campaignName", campaignName),
			zap.String("scpName", scpName), zap.String("loginName", loginName))

		return c.NoContent(http.StatusBadRequest)
	}
}

func validateBug(bugToValidate types.BugStruct) (err error) {
	if len(bugToValidate.Campaign) == 0 {
		err = fmt.Errorf("bug is not valid, empty campaign: bug: %+v", bugToValidate)
	} else if len(bugToValidate.Category) == 0 {
		err = fmt.Errorf("bug is not valid, empty category: bug: %+v", bugToValidate)
	} else if bugToValidate.PointValue < 0 {
		err = fmt.Errorf("bug is not valid, negative PointValue: bug: %+v", bugToValidate)
	}
	if err != nil {
		logger.Error("validateBug error", zap.Error(err))
	}
	return
}

func addBug(c echo.Context) (err error) {
	bug := types.BugStruct{}

	err = json.NewDecoder(c.Request().Body).Decode(&bug)
	if err != nil {
		logger.Error("error decoding bug body", zap.Error(err))
		return
	}

	if err = validateBug(bug); err != nil {
		return
	}

	err = postgresDB.InsertBug(&bug)
	if err != nil {
		return
	}

	creation := creationResponse{
		Id:     bug.Id,
		Object: bug,
	}
	return c.JSON(http.StatusCreated, creation)
}

func updateBug(c echo.Context) (err error) {
	campaign := c.Param(ParamCampaignName)
	category := c.Param(ParamBugCategory)
	pointValue, err := strconv.Atoi(c.Param(ParamPointValue))
	if err != nil {
		return
	}

	bug := types.BugStruct{Campaign: campaign, Category: category, PointValue: pointValue}
	if err = validateBug(bug); err != nil {
		return
	}

	logger.Debug(category)

	var rowsAffected int64
	rowsAffected, err = postgresDB.UpdateBug(&bug)
	if err != nil {
		return
	}
	if rowsAffected < 1 {
		return c.String(http.StatusNotFound, "Bug Category not found")
	}

	return c.String(http.StatusOK, "Success")
}

func getBugs(c echo.Context) (err error) {
	var bugs []types.BugStruct
	bugs, err = postgresDB.SelectBugs()
	if err != nil {
		return
	}

	return c.JSON(http.StatusOK, bugs)
}

func putBugs(c echo.Context) (err error) {
	var bugs []types.BugStruct
	err = json.NewDecoder(c.Request().Body).Decode(&bugs)
	if err != nil {
		logger.Error("error decoding bug body", zap.Error(err))
		return
	}

	var inserted []types.BugStruct
	for i, bug := range bugs {
		if err = validateBug(bug); err != nil {
			return
		}

		// fix warning: Implicit memory aliasing in for loop.
		//err = postgresDB.InsertBug(&bug)
		err = postgresDB.InsertBug(&bugs[i])
		if err != nil {
			logger.Error("error inserting bug", zap.Any("bug", bug), zap.Error(err))
			return
		}
		inserted = append(inserted, bugs[i])
	}

	response := creationResponse{
		Id:     inserted[0].Id,
		Object: inserted,
	}

	return c.JSON(http.StatusCreated, response)
}

func getCampaigns(c echo.Context) (err error) {
	var campaigns []types.CampaignStruct
	campaigns, err = postgresDB.GetCampaigns()
	if err != nil {
		return
	}

	return c.JSON(http.StatusOK, campaigns)
}

func getActiveCampaigns(c echo.Context) (err error) {
	current, err := postgresDB.GetActiveCampaigns(time.Now())
	if err != nil {
		return c.String(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, current)
}

func addCampaign(c echo.Context) (err error) {
	campaignName := strings.TrimSpace(c.Param(ParamCampaignName))
	if len(campaignName) == 0 {
		err = fmt.Errorf("invalid parameter %s: %s", ParamCampaignName, campaignName)
		logger.Error("addCampaign", zap.Error(err))

		return c.String(http.StatusBadRequest, err.Error())
	}

	campaignFromRequest := types.CampaignStruct{}
	err = json.NewDecoder(c.Request().Body).Decode(&campaignFromRequest)
	if err != nil {
		return
	}
	campaignFromRequest.Name = campaignName

	var guid string
	guid, err = postgresDB.InsertCampaign(&campaignFromRequest)
	if err != nil {
		return
	}

	return c.String(http.StatusCreated, guid)
}

func updateCampaign(c echo.Context) (err error) {
	campaignName := strings.TrimSpace(c.Param(ParamCampaignName))
	if len(campaignName) == 0 {
		err = fmt.Errorf("invalid parameter %s: %s", ParamCampaignName, campaignName)
		logger.Error("updateCampaign", zap.Error(err))

		return c.String(http.StatusBadRequest, err.Error())
	}

	// update campaign stored in db
	campaignFromRequest := types.CampaignStruct{}
	err = json.NewDecoder(c.Request().Body).Decode(&campaignFromRequest)
	if err != nil {
		return
	}

	// force use of path parameter campaign name value
	campaignFromRequest.Name = campaignName

	var guid string
	guid, err = postgresDB.UpdateCampaign(&campaignFromRequest)
	if err != nil {
		return
	}

	return c.String(http.StatusOK, guid)
}
