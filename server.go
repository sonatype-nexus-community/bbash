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
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sonatype-nexus-community/bbash/buildversion"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
)

var db *sql.DB

// todo remove this when upstream stuff is removed
const upstreamIdDeprecated = "deprecatedUpstreamId"

type campaignStruct struct {
	ID           string    `json:"guid"`
	Name         string    `json:"name"`
	CreatedOn    time.Time `json:"createdOn"`
	CreatedOrder int       `json:"createdOrder"`
	StartOn      time.Time `json:"startOn"`
	EndOn        time.Time `json:"endOn"`
	// todo remove this when upstream stuff is removed
	UpstreamId string         `json:"upstreamId"`
	Note       sql.NullString `json:"note"`
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

type team struct {
	Id           string `json:"guid"`
	CampaignName string `json:"campaignName"`
	Name         string `json:"name"`
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

type leaderboardCampaign struct {
	CampaignName string `json:"name"`
	Slug         string `json:"slug"`
	CreateOrder  int    `json:"create-order"`
	Active       bool   `json:"active"`
	Note         string `json:"note"`
	Archived     bool   `json:"_archived"`
	Draft        bool   `json:"_draft"`
}

type leaderboardCampaignPayload struct {
	Fields leaderboardCampaign `json:"fields"`
}

type leaderboardCampaignResponse struct {
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
var upstreamEnabled bool

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

	if upstreamEnabled {
		setupUpstream()
	}

	host, port, dbname, _, err := openDB()
	if err != nil {
		logger.Error("db open", zap.Error(err))
		panic(fmt.Errorf("failed to load database driver. host: %s, port: %d, dbname: %s, err: %+v", host, port, dbname, err))
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("db close", zap.Error(err))
		}
	}()

	err = db.Ping()
	if err != nil {
		logger.Error("db ping", zap.Error(err))
		panic(fmt.Errorf("failed to ping database. host: %s, port: %d, dbname: %s, err: %+v", host, port, dbname, err))
	}

	err = migrateDB(db)
	if err != nil {
		logger.Error("db migrate", zap.Error(err))
		panic(fmt.Errorf("failed to migrate database. err: %+v", err))
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
	publicCampaignGroup.GET(active, getActiveCampaignsEcho)

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

type CreateError struct {
	MsgPattern string
	Status     string
}

func (e *CreateError) Error() string {
	return fmt.Sprintf(e.MsgPattern, e.Status)
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
		logger.Error("error inserting organization", zap.Any("organization", organization), zap.Error(err))
		return
	}

	logger.Debug("added organization", zap.Any("organization", organization))
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
	rowsAffected, _ := res.RowsAffected()
	logger.Info("delete organization",
		zap.String("scpName", scpName),
		zap.String("orgName", orgName),
		zap.Int64("rowsAffected", rowsAffected))
	if rowsAffected > 0 {
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusNotFound, fmt.Sprintf("no organization: scpName: %s, name: %s", scpName, orgName))
}

const sqlSelectOrganizationExists = `SELECT EXISTS(
		SELECT Id FROM organization
		WHERE fk_scp = (SELECT id from source_control_provider WHERE LOWER(name) = $1) AND Organization = $2)`

// check if repo is in participating set
func validOrganization(msg scoringMessage) (orgExists bool, err error) {
	row := db.QueryRow(sqlSelectOrganizationExists, msg.EventSource, msg.RepoOwner)
	err = row.Scan(&orgExists)
	if err != nil {
		logger.Error("organization read error", zap.Any("msg", msg), zap.Error(err))
		return
	}
	if !orgExists {
		logger.Debug("organization is not valid",
			zap.String("scp", msg.EventSource), zap.String("RepoOwner", msg.RepoOwner), zap.Error(err))
	}
	return
}

const sqlSelectParticipantId = `SELECT
		participant.Id,
        campaign.name,
       	source_control_provider.name,
        participant.login_name,
        team.name
		FROM participant
		INNER JOIN campaign ON campaign.Id = fk_campaign
		INNER JOIN source_control_provider ON source_control_provider.Id = fk_scp
		LEFT JOIN team ON team.Id = participant.fk_team
		WHERE $1 >= campaign.start_on
			AND $1 < campaign.end_on
		    AND LOWER(source_control_provider.name) = $2 
			AND login_name = $3`

func validScore(msg scoringMessage, now time.Time) (participantsToScore []participant, err error) {
	// check if repo is in participating set
	isValidOrg, err := validOrganization(msg)
	if err != nil {
		logger.Debug("skip score-error reading organization", zap.Any("msg", msg), zap.Error(err))
		return
	}
	if !isValidOrg {
		logger.Debug("skip score-missing organization",
			zap.String("RepoOwner", msg.RepoOwner), zap.String("TriggerUser", msg.TriggerUser))
		return
	}

	// Check if participant is registered for an active campaign
	rows, err := db.Query(sqlSelectParticipantId, now, msg.EventSource, msg.TriggerUser)
	if err != nil {
		logger.Error("skip score-error reading participant", zap.Any("msg", msg), zap.Error(err))
		return
	}
	for rows.Next() {
		partier := participant{}
		var nullableTeamName sql.NullString
		// note: reads the db (capitalized) scpName
		err = rows.Scan(&partier.ID, &partier.CampaignName, &partier.ScpName, &partier.LoginName, &nullableTeamName)
		if nullableTeamName.Valid {
			partier.TeamName = nullableTeamName.String
		}
		if err != nil {
			logger.Error("skip score-error scanning participant", zap.Any("msg", msg), zap.Error(err))
			return
		}
		participantsToScore = append(participantsToScore, partier)
	}
	if len(participantsToScore) == 0 {
		logger.Debug("skip score-missing participant", zap.Any("msg", msg), zap.Error(err))
		return
	}
	return
}

const sqlSelectPointValue = `SELECT pointValue FROM bug 
	INNER JOIN campaign ON campaign.Id = fk_campaign	
	WHERE fk_campaign = (SELECT campaign.Id FROM campaign WHERE name = $1) 
	  AND category = $2`

func scorePoints(msg scoringMessage, campaignName string) (points int) {
	points = 0
	scored := 0

	for bugType, count := range msg.BugCounts {
		row := db.QueryRow(sqlSelectPointValue, campaignName, bugType)
		var value = 1
		if err := row.Scan(&value); err != nil {
			// ignore (and clear return) error from scan operation
			logger.Debug("ignoring missing pointValue",
				zap.String("bugType", bugType), zap.Error(err), zap.Any("msg", msg))
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
		WHERE id = $2 
		RETURNING UpstreamId, Score`

func updateParticipantScore(c echo.Context, participant participant, delta int) (err error) {
	var upstreamId string
	var score int
	row := db.QueryRow(sqlUpdateParticipantScore, delta, participant.ID)
	err = row.Scan(&upstreamId, &score)
	if err != nil {
		return
	}

	if upstreamEnabled {
		err = upstreamUpdateScore(c, upstreamId, score)
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

	//logger.Debug("scoring event", zap.Any("scoringAlert", alert))

	now := time.Now()

	for _, rawMsg := range alert.RecentHits {
		var msg scoringMessage
		err = json.Unmarshal([]byte(rawMsg), &msg)
		if err != nil {
			logger.Debug("error unmarshalling scoringMessage",
				zap.Error(err), zap.String("rawMsg", rawMsg))
			return
		}
		// force triggerUser to lower case to match database values
		msg.TriggerUser = strings.ToLower(msg.TriggerUser)

		// if this particular entry is not valid, ignore it and continue processing
		var activeParticipantsToScore []participant
		activeParticipantsToScore, err = validScore(msg, now)
		if err != nil {
			logger.Debug("error validating scoringMessage", zap.Error(err), zap.Any("msg", msg))
			return
		}
		if len(activeParticipantsToScore) == 0 {
			continue
		}
		for _, participantToScore := range activeParticipantsToScore {

			newPoints := scorePoints(msg, participantToScore.CampaignName)

			var tx *sql.Tx
			tx, err = db.Begin()
			if err != nil {
				return
			}

			row := db.QueryRow(sqlScoreQuery, participantToScore.CampaignName, participantToScore.ScpName, msg.RepoOwner, msg.RepoName, msg.PullRequest)
			oldPoints := 0
			err = row.Scan(&oldPoints)
			if err != nil {
				// ignore error case from scan when no row exists, will occur when this is a new score event
				logger.Debug("ignoring likely new score event", zap.Error(err), zap.Any("scoringMessage", msg))
			}

			_, err = db.Exec(sqlInsertScoringEvent, participantToScore.CampaignName, participantToScore.ScpName, msg.RepoOwner, msg.RepoName, msg.PullRequest, msg.TriggerUser, newPoints)
			if err != nil {
				return
			}

			err = updateParticipantScore(c, participantToScore, newPoints-oldPoints)
			if err != nil {
				return
			}

			err = tx.Commit()
			if err != nil {
				return
			}

			logger.Debug("score updated",
				zap.Int("newPoints", newPoints), zap.Int("oldPoints", oldPoints), zap.Any("scoringMessage", msg))
		}
	}

	logger.Debug("scoringAlert completed", zap.Any("alert", alert))

	return c.NoContent(http.StatusAccepted)
}

const sqlSelectParticipantDetail = `SELECT 
		participant.Id, campaign.name, source_control_provider.name, login_name, Email, DisplayName, Score, team.name, JoinedAt
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
	logger.Debug("getting detail for campaign",
		zap.String("campaignName", campaignName), zap.String("scpName", scpName), zap.String("loginName", loginName))

	row := db.QueryRow(sqlSelectParticipantDetail, campaignName, scpName, loginName)

	participant := new(participant)
	var nullableTeamName sql.NullString
	err = row.Scan(&participant.ID,
		&participant.CampaignName,
		&participant.ScpName,
		&participant.LoginName,
		&participant.Email,
		&participant.DisplayName,
		&participant.Score,
		&nullableTeamName,
		&participant.JoinedAt,
	)
	if err != nil {
		logger.Error("getParticipantDetail scan error", zap.Error(err))
		return
	}
	if nullableTeamName.Valid {
		participant.TeamName = nullableTeamName.String
	}

	return c.JSON(http.StatusOK, participant)
}

const sqlSelectParticipantsByCampaign = `SELECT
		participant.Id, campaign.name, source_control_provider.name, login_name, Email, DisplayName, Score, team.name, JoinedAt 
		FROM participant
		LEFT JOIN team ON participant.fk_team = team.Id
		INNER JOIN campaign ON participant.fk_campaign = campaign.Id
		INNER JOIN source_control_provider ON participant.fk_scp = source_control_provider.Id
		WHERE campaign.name = $1`

func getParticipantsList(c echo.Context) (err error) {
	campaignName := c.Param(ParamCampaignName)
	logger.Debug("Getting participant list for campaign: ", zap.String("campaignName", campaignName))

	rows, err := db.Query(sqlSelectParticipantsByCampaign, campaignName)
	if err != nil {
		return
	}

	var participants []participant
	for rows.Next() {
		participant := new(participant)
		var nullableTeamName sql.NullString
		err = rows.Scan(
			&participant.ID,
			&participant.CampaignName,
			&participant.ScpName,
			&participant.LoginName,
			&participant.Email,
			&participant.DisplayName,
			&participant.Score,
			&nullableTeamName,
			&participant.JoinedAt,
		)
		if err != nil {
			return
		}
		if nullableTeamName.Valid {
			participant.TeamName = nullableTeamName.String
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
		    fk_team = (SELECT Id FROM team WHERE name = $7)		    
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
		participant.TeamName,
		participant.ID,
	)
	if err != nil {
		return
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return
	}

	// @todo updateParticipantScore() only done here to update upstream. can probably remove later
	err = updateParticipantScore(c, participant, 0)
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

const sqlDeleteParticipant = `DELETE FROM participant WHERE
                          fk_campaign = (SELECT id from campaign where name =$1)
                          AND fk_scp = (SELECT id from source_control_provider where name =$2)
                          AND login_name = $3
                          RETURNING upstreamid`

func deleteParticipant(c echo.Context) (err error) {
	campaign := c.Param(ParamCampaignName)
	scpName := c.Param(ParamScpName)
	loginName := c.Param(ParamLoginName)

	var participantUpstreamId string
	err = db.QueryRow(sqlDeleteParticipant, campaign, scpName, loginName).Scan(&participantUpstreamId)
	if err != nil {
		logger.Error("error deleting participant",
			zap.String("campaign", campaign), zap.String("scpName", scpName),
			zap.String("loginName", loginName), zap.Error(err))
		return
	}

	if upstreamEnabled {
		_, err = upstreamDeleteParticipant(c, participantUpstreamId)
		if err != nil {
			return
		}
	}

	return c.JSON(http.StatusOK, fmt.Sprintf("deleted participant: campaign: %s, scpName: %s, loginName: %s, participantUpstreamId: %s",
		campaign, scpName, loginName, participantUpstreamId))
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
		logger.Error("error calling addParticipant", zap.Error(err))
	}
	return
}

func addParticipant(c echo.Context) (err error) {
	participant := participant{}

	err = json.NewDecoder(c.Request().Body).Decode(&participant)
	if err != nil {
		return
	}

	// @todo remove deprecated upstream stuff
	var webflowId string
	if upstreamEnabled {
		webflowId, err = upstreamNewParticipant(c, participant)
		if err != nil {
			return
		}
	} else {
		webflowId = upstreamIdDeprecated
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
		webflowId, // @todo remove deprecated upstream stuff
	).Scan(&guid, &participant.Score, &participant.JoinedAt)
	if err != nil {
		logger.Error("error inserting participant", zap.Any("participant", participant), zap.Error(err))
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
		(fk_campaign, name)
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
		team.CampaignName,
		team.Name).Scan(&guid)
	if err != nil {
		return
	}

	return c.String(http.StatusCreated, guid)
}

const sqlUpdateParticipantTeam = `UPDATE participant 
		SET fk_team = (SELECT Id FROM team WHERE name = $1)
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

func validateBug(bugToValidate bug) (err error) {
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

const sqlInsertBug = `INSERT INTO bug
		(fk_campaign, category, pointValue)
		VALUES ((SELECT id FROM campaign WHERE name = $1), $2, $3)
		RETURNING ID`

func addBug(c echo.Context) (err error) {
	bug := bug{}

	err = json.NewDecoder(c.Request().Body).Decode(&bug)
	if err != nil {
		logger.Error("error decoding bug body", zap.Error(err))
		return
	}

	if err = validateBug(bug); err != nil {
		return
	}

	var guid string
	err = db.QueryRow(sqlInsertBug, bug.Campaign, bug.Category, bug.PointValue).Scan(&guid)
	if err != nil {
		logger.Error("error inserting bug", zap.Any("bug", bug), zap.Error(err))
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

	if err = validateBug(bug{Campaign: campaign, Category: category, PointValue: pointValue}); err != nil {
		return
	}

	logger.Debug(category)

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

const sqlSelectBug = `SELECT bug.id, campaign.name, category, pointValue FROM bug
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
		logger.Error("error decoding bug body", zap.Error(err))
		return
	}

	tx, err := db.Begin()
	if err != nil {
		return
	}
	var inserted []bug
	for _, bug := range bugs {
		if err = validateBug(bug); err != nil {
			return
		}

		err = db.QueryRow(sqlInsertBug, bug.Campaign, bug.Category, bug.PointValue).Scan(&bug.Id)
		if err != nil {
			logger.Error("error inserting bug", zap.Any("bug", bug), zap.Error(err))
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

const sqlSelectCampaigns = `SELECT ID, name, created_on, create_order, start_on, end_on, upstream_id, note FROM campaign`

func getCampaigns(c echo.Context) (err error) {
	rows, err := db.Query(
		sqlSelectCampaigns)
	if err != nil {
		return
	}

	var campaigns []campaignStruct
	for rows.Next() {
		campaign := campaignStruct{
			// todo remove this when upstream stuff is removed
			UpstreamId: upstreamIdDeprecated,
		}
		err = rows.Scan(&campaign.ID, &campaign.Name, &campaign.CreatedOn, &campaign.CreatedOrder, &campaign.StartOn, &campaign.EndOn, &campaign.UpstreamId, &campaign.Note)
		if err != nil {
			return
		}
		campaigns = append(campaigns, campaign)
	}

	return c.JSON(http.StatusOK, campaigns)
}

const sqlSelectCampaign = `SELECT ID, name, created_on, create_order, start_on, end_on, upstream_id, note 
	FROM campaign
	WHERE name = $1`

func getCampaign(campaignName string) (campaign campaignStruct, err error) {
	rows, err := db.Query(sqlSelectCampaign, campaignName)
	if err != nil {
		return
	}

	for rows.Next() {
		err = rows.Scan(&campaign.ID, &campaign.Name, &campaign.CreatedOn, &campaign.CreatedOrder, &campaign.StartOn, &campaign.EndOn, &campaign.UpstreamId, &campaign.Note)
		if err != nil {
			return
		}
	}
	return
}

const sqlSelectCurrentCampaigns = `SELECT * FROM campaign
		WHERE $1 >= start_on
			AND $1 < end_on
		ORDER BY start_on`

func getActiveCampaigns(now time.Time) (activeCampaigns []campaignStruct, err error) {
	rows, err := db.Query(sqlSelectCurrentCampaigns, now)
	if err != nil {
		return
	}

	for rows.Next() {
		activeCampaign := campaignStruct{
			// todo remove this when upstream stuff is removed
			UpstreamId: upstreamIdDeprecated,
		}

		err = rows.Scan(&activeCampaign.ID, &activeCampaign.Name, &activeCampaign.CreatedOn, &activeCampaign.CreatedOrder, &activeCampaign.StartOn, &activeCampaign.EndOn, &activeCampaign.UpstreamId, &activeCampaign.Note)
		if err != nil {
			return
		}
		activeCampaigns = append(activeCampaigns, activeCampaign)
	}

	return
}

func getActiveCampaignsEcho(c echo.Context) (err error) {
	current, err := getActiveCampaigns(time.Now())
	if err != nil {
		return c.String(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, current)
}

const sqlInsertCampaign = `INSERT INTO campaign 
		(name, upstream_id, start_on, end_on) 
		VALUES ($1, $2, $3, $4)
		RETURNING Id`

func addCampaign(c echo.Context) (err error) {
	campaignName := strings.TrimSpace(c.Param(ParamCampaignName))
	if len(campaignName) == 0 {
		err = fmt.Errorf("invalid parameter %s: %s", ParamCampaignName, campaignName)
		logger.Error("addCampaign", zap.Error(err))

		return c.String(http.StatusBadRequest, err.Error())
	}

	campaignFromRequest := campaignStruct{}
	err = json.NewDecoder(c.Request().Body).Decode(&campaignFromRequest)
	if err != nil {
		return
	}
	campaignFromRequest.Name = campaignName

	// @todo remove deprecated upstream stuff
	var webflowId string
	if upstreamEnabled {
		webflowId, err = createNewWebflowId(c, &campaignFromRequest)
		if err != nil {
			return
		}
	} else {
		webflowId = upstreamIdDeprecated
	}

	var guid string
	err = db.QueryRow(
		sqlInsertCampaign,
		campaignName,
		webflowId, // @todo remove deprecated upstream stuff
		campaignFromRequest.StartOn,
		campaignFromRequest.EndOn,
	).Scan(&guid)
	if err != nil {
		return
	}

	return c.String(http.StatusCreated, guid)
}

const sqlUpdateCampaign = `UPDATE campaign
		SET start_on = $1,
			end_on = $2		
		WHERE name = $3
		RETURNING id`

func updateCampaign(c echo.Context) (err error) {
	campaignName := strings.TrimSpace(c.Param(ParamCampaignName))
	if len(campaignName) == 0 {
		err = fmt.Errorf("invalid parameter %s: %s", ParamCampaignName, campaignName)
		logger.Error("updateCampaign", zap.Error(err))

		return c.String(http.StatusBadRequest, err.Error())
	}

	// update campaign stored in db
	campaignFromRequest := campaignStruct{}
	err = json.NewDecoder(c.Request().Body).Decode(&campaignFromRequest)
	if err != nil {
		return
	}
	var guid string
	err = db.QueryRow(
		sqlUpdateCampaign,
		campaignFromRequest.StartOn,
		campaignFromRequest.EndOn,
		campaignName,
	).Scan(&guid)
	if err != nil {
		return
	}

	if upstreamEnabled {
		err = updateUpstreamCampaignActiveStatus(c, campaignName)
		if err != nil {
			return
		}
	}

	return c.String(http.StatusOK, guid)
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
	} else {
		logger.Info("db migration complete")
	}

	return
}

func migrateErrorApplicable(err error) bool {
	if err == nil || err == migrate.ErrNoChange {
		return false
	}
	return true
}
