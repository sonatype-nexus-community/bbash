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
	"strconv"
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

func setupMockPostgresWithInstance(mock sqlmock.Sqlmock) (args []driver.Value) {
	// mocks for 'postgres.WithInstance()'
	mock.ExpectQuery(`SELECT CURRENT_DATABASE()`).
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).FromCSVString("theDatabaseName"))
	mock.ExpectQuery(`SELECT CURRENT_SCHEMA()`).
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).FromCSVString("theDatabaseSchema"))

	args = []driver.Value{"1014225327"}
	mock.ExpectExec(`SELECT pg_advisory_lock\(\$1\)`).
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS "schema_migrations" \(version bigint not null primary key, dirty boolean not null\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectExec(`SELECT pg_advisory_unlock\(\$1\)`).
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 0))
	return
}

func TestMigrateDBErrorMigrateUp(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()

	setupMockPostgresWithInstance(mock)

	assert.EqualError(t, migrateDB(dbMock), "try lock failed in line 0: SELECT pg_advisory_lock($1) (details: all expectations were already fulfilled, call to ExecQuery 'SELECT pg_advisory_lock($1)' with args [{Name: Ordinal:1 Value:1014225327}] was not expected)")
}

func xxxTestMigrateDB(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()

	args := setupMockPostgresWithInstance(mock)

	// mocks for the migrate.Up()
	mock.ExpectExec(`SELECT pg_advisory_lock\(\$1\)`).
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery(`SELECT version, dirty FROM "schema_migrations" LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"version", "dirty"}).FromCSVString("-1,false"))

	mock.ExpectBegin()
	mock.ExpectExec(`TRUNCATE "schema_migrations"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO "schema_migrations" \(version, dirty\) VALUES \(\$1, \$2\)`).
		WithArgs(1, true).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	mock.ExpectExec(`BEGIN; CREATE EXTENSION pgcrypto; CREATE TABLE teams*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectBegin()
	mock.ExpectExec(`TRUNCATE "schema_migrations"`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO "schema_migrations" \(version, dirty\) VALUES \(\$1, \$2\)`).
		WithArgs(1, false).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	mock.ExpectExec(`SELECT pg_advisory_unlock\(\$1\)`).
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 0))

	assert.NoError(t, migrateDB(dbMock))
}

func setupMockContextCampaign(campaignName string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s/%s", CAMPAIGN, ADD, PARAM_CAMPAIGN_NAME), nil)
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

func TestAddCampaignScanError(t *testing.T) {
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

	forcedError := fmt.Errorf("forced Scan error")
	mock.ExpectQuery(`INSERT INTO campaigns \(CampaignName\) VALUES \(\$1\) RETURNING Id`).
		WithArgs(campaignName).
		WillReturnError(forcedError)

	assert.EqualError(t, addCampaign(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
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
	mock.ExpectQuery(`INSERT INTO campaigns \(CampaignName\) VALUES \(\$1\) RETURNING Id`).
		WithArgs(campaignName).
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).FromCSVString(campaignUUID))

	assert.NoError(t, addCampaign(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, campaignUUID, rec.Body.String())
}

func setupMockContextParticipant(participantJson string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s", PARTICIPANT, ADD), strings.NewReader(participantJson))
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
	participantJson := `{"gitHubName": "` + participantName + `"}`
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
	mock.ExpectQuery(`INSERT INTO participants \(GithubName, Email, DisplayName, Score, Campaign\) VALUES \(\$1\, \$2, \$3, \$4, \(SELECT Id FROM campaigns WHERE CampaignName = \$5\)\) RETURNING Id, Score, JoinedAt`).
		WillReturnError(forcedError)

	assert.EqualError(t, addParticipant(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddParticipant(t *testing.T) {
	participantName := "partName"
	participantJson := `{"gitHubName": "` + participantName + `"}`
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
	mock.ExpectQuery(`INSERT INTO participants \(GithubName, Email, DisplayName, Score, Campaign\) VALUES \(\$1\, \$2, \$3, \$4, \(SELECT Id FROM campaigns WHERE CampaignName = \$5\)\) RETURNING Id, Score, JoinedAt`).
		WithArgs(participantName, "", "", 0, "").
		WillReturnRows(sqlmock.NewRows([]string{"Id", "Score", "JoinedAt"}).AddRow(participantID, 0, time.Time{}))

	assert.NoError(t, addParticipant(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `{"guid":"`+participantID+`","endpoints":{"participantDetail"`), rec.Body.String())
	assert.True(t, strings.Contains(rec.Body.String(), `"gitHubName":"`+participantName+`"`), rec.Body.String())
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
	teamJson := `{"teamName": "` + teamName + `"}`
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
	mock.ExpectQuery(`INSERT INTO teams \(TeamName, Organization\) VALUES \(\$1\, \$2\) RETURNING Id`).
		WillReturnError(forcedError)

	assert.EqualError(t, addTeam(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddTeamEmptyOrganization(t *testing.T) {
	teamName := "myTeamName"
	teamJson := `{"teamName": "` + teamName + `"}`
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
	mock.ExpectQuery(`INSERT INTO teams \(TeamName, Organization\) VALUES \(\$1\, \$2\) RETURNING Id`).
		WithArgs(teamName, sql.NullString{}).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow(teamID))

	assert.NoError(t, addTeam(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, teamID, rec.Body.String())
}

func TestAddTeamOrganizationAsText(t *testing.T) {
	teamName := "myTeamName"
	organizationName := "myOrgName"
	teamJson := `{"teamName":"` + teamName + `", "organization": "` + organizationName + `"}`
	c, rec := setupMockContextTeam(teamJson)

	assert.EqualError(t, addTeam(c), "json: cannot unmarshal string into Go struct field team.organization of type sql.NullString")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddTeam(t *testing.T) {
	teamName := "myTeamName"
	organizationName := "myOrgName"
	teamJson := `{"teamName":"` + teamName + `", "organization": {"String":"` + organizationName + `","Valid":true}}`
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
	mock.ExpectQuery(`INSERT INTO teams \(TeamName, Organization\) VALUES \(\$1\, \$2\) RETURNING Id`).
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
	mock.ExpectExec(`UPDATE participants SET fk_team = \(SELECT Id FROM teams WHERE TeamName = \$1\) WHERE GitHubName = \$2`).
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
	mock.ExpectExec(`UPDATE participants SET fk_team = \(SELECT Id FROM teams WHERE TeamName = \$1\) WHERE GitHubName = \$2`).
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

	mock.ExpectExec(`UPDATE participants SET fk_team = \(SELECT Id FROM teams WHERE TeamName = \$1\) WHERE GitHubName = \$2`).
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

	mock.ExpectExec(`UPDATE participants SET fk_team = \(SELECT Id FROM teams WHERE TeamName = \$1\) WHERE GitHubName = \$2`).
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
	mock.ExpectQuery(`SELECT participants.Id, GitHubName, Email, DisplayName, Score, teams.TeamName, JoinedAt, campaigns.CampaignName FROM participants LEFT JOIN teams ON teams.Id = participants.fk_team INNER JOIN campaigns ON campaigns.Id = participants.Campaign WHERE participants.GitHubName = \$1`).
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
	mock.ExpectQuery(`SELECT participants.Id, GitHubName, Email, DisplayName, Score, teams.TeamName, JoinedAt, campaigns.CampaignName FROM participants LEFT JOIN teams ON teams.Id = participants.fk_team INNER JOIN campaigns ON campaigns.Id = participants.Campaign WHERE participants.GitHubName = \$1`).
		WithArgs(githubName).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "GHName", "Email", "DisplayName", "Score", "TeamName", "JoinedAt", "CampaignName"}).AddRow(participantID, githubName, "", "", "", "", time.Time{}, ""))

	assert.NoError(t, getParticipantDetail(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `{"guid":"`+participantID+`","gitHubName":"`+githubName+`"`), rec.Body.String())
}

func setupMockContextParticipantList(campaignName string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s/%s", PARTICIPANT, LIST, PARAM_CAMPAIGN_NAME), nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	c.SetParamNames(PARAM_CAMPAIGN_NAME)
	c.SetParamValues(campaignName)

	return
}

func TestGetParticipantsListScanError(t *testing.T) {
	campaignName := ""
	c, rec := setupMockContextParticipantList(campaignName)

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
	mock.ExpectQuery(`SELECT participants.Id, GitHubName, Email, DisplayName, Score, teams.TeamName, JoinedAt, campaigns.CampaignName FROM participants LEFT JOIN teams ON participants.fk_team = teams.Id INNER JOIN campaigns ON participants.Campaign = campaigns.Id WHERE campaigns.CampaignName = \$1`).
		WithArgs("").
		WillReturnError(forcedError)

	assert.EqualError(t, getParticipantsList(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetParticipantsListRowScanError(t *testing.T) {
	campaignName := ""
	c, rec := setupMockContextParticipantList(campaignName)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(`SELECT participants.Id, GitHubName, Email, DisplayName, Score, teams.TeamName, JoinedAt, campaigns.CampaignName FROM participants LEFT JOIN teams ON participants.fk_team = teams.Id INNER JOIN campaigns ON participants.Campaign = campaigns.Id WHERE campaigns.CampaignName = \$1`).
		WithArgs("").
		WillReturnRows(sqlmock.NewRows([]string{"Id", "GHName", "Email", "DisplayName", "Score", "TeamName", "JoinedAt", "CampaignName"}).
			// force scan error due to time.Time type mismatch at JoinedAt column
			AddRow(-1, "", "", "", "", "", "", campaignName))

	assert.EqualError(t, getParticipantsList(c), `sql: Scan error on column index 6, name "JoinedAt": unsupported Scan, storing driver.Value type string into type *time.Time`)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetParticipantsList(t *testing.T) {
	campaignName := "myCampaignName"
	c, rec := setupMockContextParticipantList(campaignName)

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
	mock.ExpectQuery(`SELECT participants.Id, GitHubName, Email, DisplayName, Score, teams.TeamName, JoinedAt, campaigns.CampaignName FROM participants LEFT JOIN teams ON participants.fk_team = teams.Id INNER JOIN campaigns ON participants.Campaign = campaigns.Id WHERE campaigns.CampaignName = \$1`).
		WithArgs(campaignName).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "GHName", "Email", "DisplayName", "Score", "TeamName", "JoinedAt", "CampaignName"}).
			AddRow(participantID, "", "", "", "", "", time.Time{}, campaignName))

	assert.NoError(t, getParticipantsList(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `[{"guid":"`+participantID+`","gitHubName":""`), rec.Body.String())
	assert.True(t, strings.HasSuffix(rec.Body.String(), `","campaignName":"`+campaignName+`"}]`+"\n"), rec.Body.String())
}

func setupMockContextAddBug(bugJson string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s", BUG, ADD), strings.NewReader(bugJson))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	return
}

func TestAddBugMissingBug(t *testing.T) {
	c, rec := setupMockContextAddBug("")

	assert.EqualError(t, addBug(c), "EOF")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddBugScanError(t *testing.T) {
	category := "myCategory"
	c, rec := setupMockContextAddBug(`{"category":"` + category + `"}`)

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
	mock.ExpectQuery(`INSERT INTO bugs \(category, pointValue\) VALUES \(\$1, \$2\) RETURNING ID`).
		WithArgs(category, 0).
		WillReturnError(forcedError)

	assert.EqualError(t, addBug(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddBug(t *testing.T) {
	category := "myCategory"
	pointValue := 9
	c, rec := setupMockContextAddBug(`{"category":"` + category + `","pointValue":` + strconv.Itoa(pointValue) + `}`)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	bugId := "myBugId"
	mock.ExpectQuery(`INSERT INTO bugs \(category, pointValue\) VALUES \(\$1, \$2\) RETURNING ID`).
		WithArgs(category, pointValue).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).
			AddRow(bugId))

	assert.NoError(t, addBug(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `{"guid":"`+bugId+`","endpoints":`), rec.Body.String())
	assert.True(t, strings.HasSuffix(rec.Body.String(), `"object":{"guid":"`+bugId+`","category":"`+category+`","pointValue":`+strconv.Itoa(pointValue)+`}}`+"\n"), rec.Body.String())
}

func setupMockContextUpdateBug(bugCategory, pointValue string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("%s/%s/%s/%s", BUG, UPDATE, PARAM_BUG_CATEGORY, PARAM_POINT_VALUE), nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	c.SetParamNames(PARAM_BUG_CATEGORY, PARAM_POINT_VALUE)
	c.SetParamValues(bugCategory, pointValue)

	return
}

func TestUpdateBugInvalidPointValue(t *testing.T) {
	c, rec := setupMockContextUpdateBug("", "non-number")

	assert.EqualError(t, updateBug(c), `strconv.Atoi: parsing "non-number": invalid syntax`)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateBugUpdateError(t *testing.T) {
	category := "myCategory"
	pointValue := 9
	c, rec := setupMockContextUpdateBug(category, strconv.Itoa(pointValue))

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced Update error")
	mock.ExpectExec(`UPDATE bugs SET pointValue = \$1 WHERE category = \$2`).
		WithArgs(pointValue, category).
		WillReturnError(forcedError)

	assert.EqualError(t, updateBug(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateBugRowsAffectedError(t *testing.T) {
	category := "myCategory"
	pointValue := 9
	c, rec := setupMockContextUpdateBug(category, strconv.Itoa(pointValue))

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
	mock.ExpectExec(`UPDATE bugs SET pointValue = \$1 WHERE category = \$2`).
		WithArgs(pointValue, category).
		WillReturnResult(sqlmock.NewErrorResult(forcedError))

	assert.EqualError(t, updateBug(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateBugRowsAffectedZero(t *testing.T) {
	category := "myCategory"
	pointValue := 9
	c, rec := setupMockContextUpdateBug(category, strconv.Itoa(pointValue))

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectExec(`UPDATE bugs SET pointValue = \$1 WHERE category = \$2`).
		WithArgs(pointValue, category).
		WillReturnResult(sqlmock.NewResult(0, 0))

	assert.NoError(t, updateBug(c))
	assert.Equal(t, http.StatusNotFound, c.Response().Status)
	assert.Equal(t, "Bug Category not found", rec.Body.String())
}

func TestUpdateBug(t *testing.T) {
	category := "myCategory"
	pointValue := 9
	c, rec := setupMockContextUpdateBug(category, strconv.Itoa(pointValue))

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectExec(`UPDATE bugs SET pointValue = \$1 WHERE category = \$2`).
		WithArgs(pointValue, category).
		WillReturnResult(sqlmock.NewResult(0, 5))

	assert.NoError(t, updateBug(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.Equal(t, "Success", rec.Body.String())
}

func setupMockContextGetBugs() (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", BUG, LIST), nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	return
}

func TestGetBugsSelectError(t *testing.T) {
	c, rec := setupMockContextGetBugs()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced Select error")
	mock.ExpectQuery(`SELECT \* FROM bugs`).WillReturnError(forcedError)

	assert.EqualError(t, getBugs(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetBugsScanError(t *testing.T) {
	c, rec := setupMockContextGetBugs()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(`SELECT \* FROM bugs`).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "Category", "PointValue"}).
			// force scan error due to time.Time type mismatch at PointValue column
			AddRow(-1, "", "non-number"))

	assert.EqualError(t, getBugs(c), `sql: Scan error on column index 2, name "PointValue": converting driver.Value type string ("non-number") to a int: invalid syntax`)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetBugs(t *testing.T) {
	c, rec := setupMockContextGetBugs()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	bugId := "myBugId"
	category := "myCategory"
	pointValue := 9
	mock.ExpectQuery(`SELECT \* FROM bugs`).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "Category", "PointValue"}).
			// force scan error due to time.Time type mismatch at PointValue column
			AddRow(bugId, category, strconv.Itoa(pointValue)))

	assert.NoError(t, getBugs(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.Equal(t, `[{"guid":"`+bugId+`","category":"`+category+`","pointValue":`+strconv.Itoa(pointValue)+`}]`+"\n", rec.Body.String())
}

func setupMockContextPutBugs(bugsJson string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s", BUG, LIST), strings.NewReader(bugsJson))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	return
}

func TestPutBugsBodyInvalid(t *testing.T) {
	c, rec := setupMockContextPutBugs("")

	assert.EqualError(t, putBugs(c), "EOF")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestPutBugsBeginTxError(t *testing.T) {
	c, rec := setupMockContextPutBugs(`[{}]`)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced Begin Txn error")
	mock.ExpectBegin().WillReturnError(forcedError)

	assert.EqualError(t, putBugs(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestPutBugsScanError(t *testing.T) {
	c, rec := setupMockContextPutBugs(`[{}]`)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectBegin()
	forcedError := fmt.Errorf("forced Scan error")
	mock.ExpectQuery(`INSERT INTO bugs \(category, pointValue\) VALUES \(\$1, \$2\) RETURNING ID`).
		WithArgs("", 0).
		WillReturnError(forcedError)

	assert.EqualError(t, putBugs(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestPutBugsCommitTxError(t *testing.T) {
	c, rec := setupMockContextPutBugs(`[{}]`)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO bugs \(category, pointValue\) VALUES \(\$1, \$2\) RETURNING ID`).
		WithArgs("", 0).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).
			AddRow(""))
	forcedError := fmt.Errorf("forced Commit Txn error")
	mock.ExpectCommit().WillReturnError(forcedError)

	assert.EqualError(t, putBugs(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestPutBugsOneBug(t *testing.T) {
	c, rec := setupMockContextPutBugs(`[{}]`)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectBegin()
	bugId := "myBugId"
	mock.ExpectQuery(`INSERT INTO bugs \(category, pointValue\) VALUES \(\$1, \$2\) RETURNING ID`).
		WithArgs("", 0).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).
			AddRow(bugId))
	mock.ExpectCommit()

	assert.NoError(t, putBugs(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, `{"guid":"`+bugId+`","endpoints":null,"object":[{"guid":"`+bugId+`","category":"","pointValue":0}]}`+"\n", rec.Body.String())
}

func TestPutBugsMultipleBugs(t *testing.T) {
	c, rec := setupMockContextPutBugs(`[{}, {}]`)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectBegin()
	bugId := "myBugId"
	mock.ExpectQuery(`INSERT INTO bugs \(category, pointValue\) VALUES \(\$1, \$2\) RETURNING ID`).
		WithArgs("", 0).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).
			AddRow(bugId))

	bugId2 := "secondBugId"
	mock.ExpectQuery(`INSERT INTO bugs \(category, pointValue\) VALUES \(\$1, \$2\) RETURNING ID`).
		WithArgs("", 0).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).
			AddRow(bugId2))
	mock.ExpectCommit()

	assert.NoError(t, putBugs(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, `{"guid":"`+bugId+`","endpoints":null,"object":[{"guid":"`+bugId+`","category":"","pointValue":0},{"guid":"`+bugId2+`","category":"","pointValue":0}]}`+"\n", rec.Body.String())
}
