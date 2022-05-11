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
	"github.com/sonatype-nexus-community/bbash/internal/poll"
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
var scoreDB db.IScoreDB
var pollDB db.IDBPoll

type creationResponse struct {
	Id        string                 `json:"guid"`
	Endpoints map[string]interface{} `json:"endpoints"`
	Object    interface{}            `json:"object"`
}

type endpointDetail struct {
	URI  string `json:"uri"`
	Verb string `json:"httpVerb"`
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
	Poll                  string = "/poll"
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
const envLogFilterIncludeHostname = "LOG_FILTER_INCLUDE_HOSTNAME"

var errRecovered error
var logger *zap.Logger

var stopPoll chan bool

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

	scoreDB = postgresDB
	if os.Getenv("DISABLE_DATADOG_POLL") == "" {
		// polling voodoo
		var errChan chan error
		stopPoll, errChan, err = beginLogPolling()
		if err != nil {
		    logger.Error("begin polling", zap.Error(err))
		    panic(fmt.Errorf("failed to start polling. err: %+v", err))
		}

		defer func() {
			close(stopPoll)
			pollErr := <-errChan
			logger.Error("defer poll error", zap.Error(pollErr))
		}()
	}

	logger.Fatal("application end", zap.Error(e.Start(defaultServicePort)))
}

func beginLogPolling() (quit chan bool, errChan chan error, err error) {
	err = godotenv.Load(".env.dd")
	if err != nil {
		logger.Error(".env.dd load error", zap.Error(err))
		// clear error from load .env.dd file
		err = nil
	}

	var pollDogIntervalSeconds int
	pollDogIntervalSeconds, err = strconv.Atoi(os.Getenv("DD_CLIENT_POLL_SECONDS"))
	if err != nil {
		pollDogIntervalSeconds = 120
		logger.Info("missing env var DD_CLIENT_POLL_SECONDS, using default",
			zap.Int("pollDogIntervalSeconds", pollDogIntervalSeconds),
			zap.Error(err),
		)
		// clear error from read env var
		err = nil
	}

	pollDB = db.NewDBPoll(scoreDB.GetDb(), logger)
	quit, errChan = poll.ChaseTail(pollDB, scoreDB, time.Duration(pollDogIntervalSeconds), processScoringMessage)
	return
}

//goland:noinspection GoUnusedParameter
func restartPolling(c echo.Context) (err error) {
	if stopPoll != nil {
		close(stopPoll)
	}
	stopPoll, _, err = beginLogPolling()
	return
}

//goland:noinspection GoUnusedParameter
func stopPolling(c echo.Context) (err error) {
	close(stopPoll)
	stopPoll = nil
	return
}

func setPollDate(c echo.Context) (err error) {
	pollFromRequest := types.Poll{}
	err = json.NewDecoder(c.Request().Body).Decode(&pollFromRequest)
	if err != nil {
		return
	}

	pollFromDb := pollDB.NewPoll()
	err = pollDB.SelectPoll(&pollFromDb)
	if err != nil {
		return
	}

	pollFromDb.LastPolled = pollFromRequest.LastPolled
	err = pollDB.UpdatePoll(&pollFromDb)
	if err != nil {
		return
	}

	logger.Info("set poll", zap.Any("poll", pollFromDb))
	return
}

func setupRoutes(e *echo.Echo, buildInfoMessage string) (customRouteCount int) {
	e.GET("/health", func(c echo.Context) error {
		return c.String(http.StatusOK, fmt.Sprintf("I am ALIVE. %s", buildInfoMessage))
	})

	// admin endpoint group
	adminGroup := e.Group(pathAdmin, middleware.BasicAuth(infoBasicValidator))

	// Source Control Provider endpoints
	scpGroup := adminGroup.Group(SourceControlProvider)
	scpGroup.GET(List, getSourceControlProviders).Name = "scp-list"

	// Organization related endpoints
	organizationGroup := adminGroup.Group(Organization)

	organizationGroup.GET(List, getOrganizations).Name = "organization-list"
	organizationGroup.PUT(Add, addOrganization).Name = "organization-add"
	organizationGroup.DELETE(
		fmt.Sprintf("%s/:%s/:%s", Delete, ParamScpName, ParamOrganizationName),
		deleteOrganization).Name = "organization-delete"

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

	// Poll related endpoints and group

	pollGroup := adminGroup.Group(Poll)
	pollGroup.PUT("/last", setPollDate)
	pollGroup.DELETE("/stop", stopPolling)
	pollGroup.GET("/restart", restartPolling)

	e.Static("/", buildLocation)

	routes := e.Routes()

	for _, v := range routes {
		routeInfo := fmt.Sprintf("%s %s as %s", v.Method, v.Path, v.Name)
		// only print the routes we created ourselves, ignoring the default ones added automatically by echo
		if !strings.HasPrefix(v.Name, echoDefaultRouteNamePrefix) {
			customRouteCount++
			logger.Info("route", zap.String("info", routeInfo))
		}
	}
	return
}

const echoDefaultRouteNamePrefix = "github.com/labstack/echo/v4."

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
				return err
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

			logIncludeHostname := os.Getenv(envLogFilterIncludeHostname)
			if logIncludeHostname != "" && req.Host != "" {
				// only log legit stuff from expected host
				if logIncludeHostname != req.Host {
					return nil
				}
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
		logger.Error("error inserting organization", zap.Any("organization", organization), zap.Error(err))
		return
	}

	logger.Debug("added organization", zap.Any("organization", organization))
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
	logger.Info("delete organization",
		zap.String("scpName", scpName),
		zap.String("orgName", orgName),
		zap.Int64("rowsAffected", rowsAffected))
	if rowsAffected > 0 {
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusNotFound, fmt.Sprintf("no organization: scpName: %s, name: %s", scpName, orgName))
}

func validScore(msg *types.ScoringMessage, now time.Time) (participantsToScore []types.ParticipantStruct, err error) {
	// check if repo is in participating set
	isValidOrg, err := postgresDB.ValidOrganization(msg)
	if err != nil {
		logger.Debug("skip score-error reading organization", zap.Any("scoringMsg", msg), zap.Error(err))
		return
	}
	if !isValidOrg {
		logger.Debug("skip score-missing organization",
			zap.String("RepoOwner", msg.RepoOwner), zap.String("TriggerUser", msg.TriggerUser))
		return
	}

	// Check if participant is registered for an active campaign
	participantsToScore, err = postgresDB.SelectParticipantsToScore(msg, now)
	if err != nil {
		logger.Error("skip score-error reading participant", zap.Any("scoringMsg", msg), zap.Error(err))
		return
	}
	if len(participantsToScore) == 0 {
		logger.Debug("skip score-missing participant", zap.Any("scoringMsg", msg), zap.Error(err))
		return
	}
	return
}

func scorePoints(msg *types.ScoringMessage, campaignName string) (points float64) {
	points = 0
	scored := float64(0)

	err := traverseBugCounts(msg, campaignName, &points, &scored, &msg.BugCounts)
	if err != nil {
		logger.Error("error traversing bugCounts", zap.Error(err), zap.Any("scoringMsg", msg))
	}

	// add 1 point for all non-classified fixed bugs
	if scored < float64(msg.TotalFixed) {
		points += float64(msg.TotalFixed) - scored
	}

	return
}

func traverseBugCounts(msg *types.ScoringMessage, campaignName string,
	points, scored *float64, bugTypes *map[string]interface{}) (err error) {

	for bugType, bugValue := range *bugTypes {
		switch v := bugValue.(type) {
		case float64:
			value := postgresDB.SelectPointValue(msg, campaignName, bugType)
			*points += v * value
			*scored += v
		case map[string]interface{}:
			// oh joy, recursion.
			err = traverseBugCounts(msg, campaignName, points, scored, &v)
		default:
			err = fmt.Errorf("bugType: %+v has unexpected bugValue type: %+v", bugType, v)
			logger.Error("traverseBugCounts", zap.Error(err), zap.Any("scoringMsg", msg))
		}
	}
	return
}

func processScoringMessage(scoreDb db.IScoreDB, now time.Time, msg *types.ScoringMessage) (err error) {
	// force triggerUser to lower case to match database values
	msg.TriggerUser = strings.ToLower(msg.TriggerUser)

	// if this particular entry is not valid, ignore it and continue processing
	var activeParticipantsToScore []types.ParticipantStruct
	activeParticipantsToScore, err = validScore(msg, now)
	if err != nil {
		logger.Debug("error validating ScoringMessage", zap.Error(err), zap.Any("scoringMsg", msg))
		return
	}
	if len(activeParticipantsToScore) == 0 {
		return
	}
	for _, participantToScore := range activeParticipantsToScore {

		newPoints := scorePoints(msg, participantToScore.CampaignName)

		oldPoints := scoreDb.SelectPriorScore(&participantToScore, msg)

		err = scoreDb.InsertScoringEvent(&participantToScore, msg, newPoints)
		if err != nil {
			return
		}

		err = scoreDb.UpdateParticipantScore(&participantToScore, newPoints-oldPoints)
		if err != nil {
			return
		}

		logger.Debug("score updated",
			zap.Float64("newPoints", newPoints), zap.Float64("oldPoints", oldPoints), zap.Any("ScoringMessage", msg))
	}
	return
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
	logTelemetry(c)

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

func validateBug(bugToValidate *types.BugStruct) (err error) {
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

	if err = validateBug(&bug); err != nil {
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
	if err = validateBug(&bug); err != nil {
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
	for _, bug := range bugs {
		if err = validateBug(&bug); err != nil {
			return
		}

		err = postgresDB.InsertBug(&bug)
		if err != nil {
			logger.Error("error inserting bug", zap.Any("bug", bug), zap.Error(err))
			return
		}
		inserted = append(inserted, bug)
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

const msgTelemetry = "log-telemetry"
const qpFeature = "feature"
const qpCall = "call"

func logTelemetry(c echo.Context) {
	feature := c.QueryParam(qpFeature)
	call := c.QueryParam(qpCall)
	if feature != "" && call != "" {
		logger.Info(msgTelemetry,
			zap.String(qpFeature, feature),
			zap.String(qpCall, call),
		)
	}
}

func getActiveCampaigns(c echo.Context) (err error) {
	logTelemetry(c)

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
