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
	"database/sql/driver"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newMockDb(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		assert.NoError(t, err)
	}

	return db, mock
}

func TestMigrateDBErrorPostgresWithInstance(t *testing.T) {
	dbMock, _ := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()

	assert.EqualError(t, migrateDB(dbMock), "all expectations were already fulfilled, call to Query 'SELECT CURRENT_DATABASE()' with args [] was not expected in line 0: SELECT CURRENT_DATABASE()")
}

func TestMigrateDBErrorMigrateUp(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()

	setupMockPostgresWithInstance(mock)

	assert.EqualError(t, migrateDB(dbMock), "try lock failed in line 0: SELECT pg_advisory_lock($1) (details: all expectations were already fulfilled, call to ExecQuery 'SELECT pg_advisory_lock($1)' with args [{Name: Ordinal:1 Value:1014225327}] was not expected)")
}

func setupMockPostgresWithInstance(mock sqlmock.Sqlmock) (args []driver.Value) {
	// mocks for 'postgres.WithInstance()'
	mock.ExpectQuery("SELECT CURRENT_DATABASE()").
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).FromCSVString("theDatabaseName"))
	mock.ExpectQuery("SELECT CURRENT_SCHEMA()").
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).FromCSVString("theDatabaseSchema"))

	args = []driver.Value{"1014225327"}
	mock.ExpectExec("SELECT pg_advisory_lock\\(\\$1\\)").
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS \"schema_migrations\" \\(version bigint not null primary key, dirty boolean not null\\)").
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 0))
	return
}

func xxxTestMigrateDB(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()

	args := setupMockPostgresWithInstance(mock)

	// mocks for the migrate.Up()
	mock.ExpectExec("SELECT pg_advisory_lock\\(\\$1\\)").
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery("SELECT version, dirty FROM \"schema_migrations\" LIMIT 1").
		WillReturnRows(sqlmock.NewRows([]string{"version", "dirty"}).FromCSVString("-1,false"))

	mock.ExpectBegin()
	mock.ExpectExec("TRUNCATE \"schema_migrations\"").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO \"schema_migrations\" \\(version, dirty\\) VALUES \\(\\$1, \\$2\\)").
		WithArgs(1, true).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	mock.ExpectExec("BEGIN; CREATE EXTENSION pgcrypto; CREATE TABLE teams*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectBegin()
	mock.ExpectExec("TRUNCATE \"schema_migrations\"").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO \"schema_migrations\" \\(version, dirty\\) VALUES \\(\\$1, \\$2\\)").
		WithArgs(1, false).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 0))

	assert.NoError(t, migrateDB(dbMock))
}

func setupMockContextCampaign(campaignName string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s", ADD, PARAM_CAMPAIGN_NAME), nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames(PARAM_CAMPAIGN_NAME)
	c.SetParamValues(campaignName)
	return
}

func TestAddCampaignEmptyName(t *testing.T) {
	campaignName := " "
	c, rec := setupMockContextCampaign(campaignName)

	dbMock, _ := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	expectedError := fmt.Errorf("invalid parameter %s: %s", PARAM_CAMPAIGN_NAME, "")

	assert.NoError(t, addCampaign(c))
	assert.Equal(t, http.StatusBadRequest, c.Response().Status)
	assert.Equal(t, expectedError.Error(), rec.Body.String())
}

func TestAddCampaign(t *testing.T) {
	campaignName := "myCampaignName"
	c, rec := setupMockContextCampaign(campaignName)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	campaignUUID := "campaignId"
	mock.ExpectQuery("INSERT INTO campaigns \\(CampaignName\\) VALUES \\(\\$1\\) RETURNING Id").
		WithArgs(campaignName).
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).FromCSVString(campaignUUID))

	assert.NoError(t, addCampaign(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, campaignUUID, rec.Body.String())
}

func setupMockContextParticipant(participantJson string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s", ADD, PARTICIPANT), strings.NewReader(participantJson))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	return
}

func TestAddParticipantBodyInvalid(t *testing.T) {
	c, rec := setupMockContextParticipant("")

	assert.EqualError(t, addParticipant(c), "EOF")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddParticipantCampaignMissing(t *testing.T) {
	participantName := "partName"
	participantJson := "{ \"gitHubName\": \"" + participantName + "\"}"
	c, rec := setupMockContextParticipant(participantJson)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced SQL insert error")
	mock.ExpectQuery("INSERT INTO participants \\(GithubName, Email, DisplayName, Score, Campaign\\) VALUES \\(\\$1\\, \\$2, \\$3, \\$4, \\(SELECT Id FROM campaigns WHERE CampaignName = \\$5\\)\\) RETURNING Id, Score, JoinedAt").
		WillReturnError(forcedError)

	assert.EqualError(t, addParticipant(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddParticipant(t *testing.T) {
	participantName := "partName"
	participantJson := "{ \"gitHubName\": \"" + participantName + "\"}"
	c, rec := setupMockContextParticipant(participantJson)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	participantID := "participantUUId"
	mock.ExpectQuery("INSERT INTO participants \\(GithubName, Email, DisplayName, Score, Campaign\\) VALUES \\(\\$1\\, \\$2, \\$3, \\$4, \\(SELECT Id FROM campaigns WHERE CampaignName = \\$5\\)\\) RETURNING Id, Score, JoinedAt").
		WithArgs(participantName, "", "", 0, "").
		WillReturnRows(sqlmock.NewRows([]string{"Id", "Score", "JoinedAt"}).AddRow(participantID, 0, time.Time{}))

	assert.NoError(t, addParticipant(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), "{\"guid\":\""+participantID+"\",\"endpoints"), rec.Body.String())
	assert.True(t, strings.Contains(rec.Body.String(), "\"gitHubName\":\""+participantName+"\""), rec.Body.String())
}

func setupMockContextTeam(teamJson string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s", TEAM, ADD), strings.NewReader(teamJson))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	return
}

func TestAddTeamMissingTeam(t *testing.T) {
	c, rec := setupMockContextTeam("")

	assert.EqualError(t, addTeam(c), "EOF")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddTeamInsertError(t *testing.T) {
	teamName := "myTeamName"
	teamJson := "{ \"teamName\": \"" + teamName + "\"}"
	c, rec := setupMockContextTeam(teamJson)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced SQL insert error")
	mock.ExpectQuery("INSERT INTO teams \\(TeamName, Organization\\) VALUES \\(\\$1\\, \\$2\\) RETURNING Id").
		WillReturnError(forcedError)

	assert.EqualError(t, addTeam(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddTeamEmptyOrganization(t *testing.T) {
	teamName := "myTeamName"
	teamJson := "{ \"teamName\": \"" + teamName + "\"}"
	c, rec := setupMockContextTeam(teamJson)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	teamID := "teamUUId"
	mock.ExpectQuery("INSERT INTO teams \\(TeamName, Organization\\) VALUES \\(\\$1\\, \\$2\\) RETURNING Id").
		WithArgs(teamName, sql.NullString{}).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow(teamID))

	assert.NoError(t, addTeam(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, teamID, rec.Body.String())
}

func TestAddTeamOrganizationAsText(t *testing.T) {
	teamName := "myTeamName"
	organizationName := "myOrgName"
	teamJson := "{ \"teamName\": \"" + teamName + "\", \"organization\": \"" + organizationName + "\"}"
	c, rec := setupMockContextTeam(teamJson)

	assert.EqualError(t, addTeam(c), "json: cannot unmarshal string into Go struct field team.organization of type sql.NullString")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddTeam(t *testing.T) {
	teamName := "myTeamName"
	organizationName := "myOrgName"
	teamJson := "{ \"teamName\": \"" + teamName + "\", \"organization\": {\"String\":\"" + organizationName + "\",\"Valid\":true}}"
	c, rec := setupMockContextTeam(teamJson)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	teamID := "teamUUId"
	mock.ExpectQuery("INSERT INTO teams \\(TeamName, Organization\\) VALUES \\(\\$1\\, \\$2\\) RETURNING Id").
		WithArgs(teamName, sql.NullString{String: organizationName, Valid: true}).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow(teamID))

	assert.NoError(t, addTeam(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, teamID, rec.Body.String())
}

func setupMockContextAddPersonToTeam(githubName, teamName string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s/%s/%s", TEAM, PERSON, PARAM_GITHUB_NAME, PARAM_TEAM_NAME), nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	c.SetParamNames(PARAM_GITHUB_NAME, PARAM_TEAM_NAME)
	c.SetParamValues(githubName, teamName)

	return
}

func TestAddPersonToTeamMissingParameters(t *testing.T) {
	c, rec := setupMockContextAddPersonToTeam("", "")

	assert.NoError(t, addPersonToTeam(c))
	assert.Equal(t, http.StatusBadRequest, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddPersonToTeamUpdateError(t *testing.T) {
	githubName := "myGithubName"
	teamName := "myTeamName"
	c, rec := setupMockContextAddPersonToTeam(githubName, teamName)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced SQL update error")
	mock.ExpectExec("UPDATE participants SET fk_team = \\(SELECT Id FROM teams WHERE TeamName = \\$1\\) WHERE GitHubName = \\$2").
		WillReturnError(forcedError)

	assert.EqualError(t, addPersonToTeam(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddPersonToTeamRowsAffectedError(t *testing.T) {
	githubName := "myGithubName"
	teamName := "myTeamName"
	c, rec := setupMockContextAddPersonToTeam(githubName, teamName)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced Rows Affected error")
	mock.ExpectExec("UPDATE participants SET fk_team = \\(SELECT Id FROM teams WHERE TeamName = \\$1\\) WHERE GitHubName = \\$2").
		WithArgs(teamName, githubName).
		WillReturnResult(sqlmock.NewErrorResult(forcedError))

	assert.EqualError(t, addPersonToTeam(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddPersonToTeamZeroRowsAffected(t *testing.T) {
	githubName := "myGithubName"
	teamName := "myTeamName"
	c, rec := setupMockContextAddPersonToTeam(githubName, teamName)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectExec("UPDATE participants SET fk_team = \\(SELECT Id FROM teams WHERE TeamName = \\$1\\) WHERE GitHubName = \\$2").
		WithArgs(teamName, githubName).
		WillReturnResult(sqlmock.NewResult(0, 0))

	assert.NoError(t, addPersonToTeam(c))
	assert.Equal(t, http.StatusBadRequest, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddPersonToTeamSomeRowsAffected(t *testing.T) {
	githubName := "myGithubName"
	teamName := "myTeamName"
	c, rec := setupMockContextAddPersonToTeam(githubName, teamName)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectExec("UPDATE participants SET fk_team = \\(SELECT Id FROM teams WHERE TeamName = \\$1\\) WHERE GitHubName = \\$2").
		WithArgs(teamName, githubName).
		WillReturnResult(sqlmock.NewResult(0, 5))

	assert.NoError(t, addPersonToTeam(c))
	assert.Equal(t, http.StatusNoContent, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func setupMockContextParticipantDetail(githubName string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s/%s", PARTICIPANT, DETAIL, PARAM_GITHUB_NAME), nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	c.SetParamNames(PARAM_GITHUB_NAME)
	c.SetParamValues(githubName)

	return
}

func TestGetParticipantDetailScanError(t *testing.T) {
	c, rec := setupMockContextParticipantDetail("")

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced Scan error")
	mock.ExpectQuery("SELECT participants.Id, GitHubName, Email, DisplayName, Score, teams.TeamName, JoinedAt, campaigns.CampaignName FROM participants LEFT JOIN teams ON teams.Id = participants.fk_team INNER JOIN campaigns ON campaigns.Id = participants.Campaign WHERE participants.GitHubName = \\$1").
		WithArgs("").
		WillReturnError(forcedError)

	assert.EqualError(t, getParticipantDetail(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetParticipantDetail(t *testing.T) {
	githubName := "myGithubName"
	c, rec := setupMockContextParticipantDetail(githubName)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	participantID := "9"
	mock.ExpectQuery("SELECT participants.Id, GitHubName, Email, DisplayName, Score, teams.TeamName, JoinedAt, campaigns.CampaignName FROM participants LEFT JOIN teams ON teams.Id = participants.fk_team INNER JOIN campaigns ON campaigns.Id = participants.Campaign WHERE participants.GitHubName = \\$1").
		WithArgs(githubName).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "GHName", "Email", "DisplayName", "Score", "TeamName", "JoinedAt", "CampaignName"}).AddRow(participantID, githubName, "", "", "", "", time.Time{}, ""))

	assert.NoError(t, getParticipantDetail(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), "{\"guid\":\""+participantID+"\",\"gitHubName\":\""+githubName+"\""), rec.Body.String())
}
