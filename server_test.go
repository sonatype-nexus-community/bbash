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
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"regexp"
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

//goland:noinspection GoUnusedFunction
func xxxTestMigrateDB(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()

	args := setupMockPostgresWithInstance(mock)

	// mocks for migrate.Up()
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
	req := httptest.NewRequest("", "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames(ParamCampaignName)
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

	expectedError := fmt.Errorf("invalid parameter %s: %s", ParamCampaignName, "")

	assert.NoError(t, addCampaign(c))
	assert.Equal(t, http.StatusBadRequest, c.Response().Status)
	assert.Equal(t, expectedError.Error(), rec.Body.String())
}

// convertSqlToDbMockExpect takes a "real" sql string and adds escape characters as needed to produce a
// regex matching string for use with database mock expect calls.
func convertSqlToDbMockExpect(realSql string) string {
	reDollarSign := regexp.MustCompile(`(\$)`)
	sqlMatch := reDollarSign.ReplaceAll([]byte(realSql), []byte(`\$`))

	reLeftParen := regexp.MustCompile(`(\()`)
	sqlMatch = reLeftParen.ReplaceAll(sqlMatch, []byte(`\(`))

	reRightParen := regexp.MustCompile(`(\))`)
	sqlMatch = reRightParen.ReplaceAll(sqlMatch, []byte(`\)`))

	reStar := regexp.MustCompile(`(\*)`)
	sqlMatch = reStar.ReplaceAll(sqlMatch, []byte(`\*`))

	rePlus := regexp.MustCompile(`(\+)`)
	sqlMatch = rePlus.ReplaceAll(sqlMatch, []byte(`\+`))
	return string(sqlMatch)
}

func TestConvertSqlToDbMockExpect(t *testing.T) {
	// sanity check all the cases we've found so far
	assert.Equal(t, `\$\(\)\*\+`, convertSqlToDbMockExpect(`$()*+`))
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertCampaign)).
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertCampaign)).
		WithArgs(campaignName).
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).FromCSVString(campaignUUID))

	assert.NoError(t, addCampaign(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, campaignUUID, rec.Body.String())
}

func setupMockContextWebflow() (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames("live")
	c.SetParamValues("true")
	return
}

func TestGetNetClient(t *testing.T) {
	netClient := getNetClient()
	assert.Equal(t, 10.0, netClient.Timeout.Seconds())
}

func TestRequestHeaderSetup(t *testing.T) {
	req, err := http.NewRequest("myMethod", "myUrl", nil)
	assert.NoError(t, err)
	requestHeaderSetup(req)
	verifyRequestHeaders(t, req)
}

func verifyRequestHeaders(t *testing.T, req *http.Request) {
	assert.Equal(t, fmt.Sprintf("Bearer %s", webflowToken), req.Header.Get("Authorization"))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Equal(t, "1.0.0", req.Header.Get("accept-version"))
}

func TestUpstreamNewParticipantWebflowErrorNotFound(t *testing.T) {
	origWebflowColletion := webflowCollection
	defer func() {
		webflowCollection = origWebflowColletion
	}()
	webflowCollection = "testWfCollection"
	origWebflowToken := webflowToken
	defer func() {
		webflowToken = origWebflowToken
	}()
	webflowToken = "testWfToken"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", webflowCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	origWebflowBaseAPI := webflowBaseAPI
	defer func() {
		webflowBaseAPI = origWebflowBaseAPI
	}()
	webflowBaseAPI = ts.URL

	c, rec := setupMockContextWebflow()

	id, err := upstreamNewParticipant(c, participant{})
	assert.Equal(t, "", id)
	expectedErr := &ParticipantCreateError{"404 Not Found"}
	assert.EqualError(t, err, expectedErr.Error())
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, expectedErr.Error(), rec.Body.String())
}

func TestUpstreamNewParticipantWebflowIDDecodeError(t *testing.T) {
	origWebflowColletion := webflowCollection
	defer func() {
		webflowCollection = origWebflowColletion
	}()
	webflowCollection = "testWfCollection"
	origWebflowToken := webflowToken
	defer func() {
		webflowToken = origWebflowToken
	}()
	webflowToken = "testWfToken"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", webflowCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("bad json text"))
	}))
	defer ts.Close()
	origWebflowBaseAPI := webflowBaseAPI
	defer func() {
		webflowBaseAPI = origWebflowBaseAPI
	}()
	webflowBaseAPI = ts.URL

	c, rec := setupMockContextWebflow()

	id, err := upstreamNewParticipant(c, participant{})
	assert.Equal(t, "", id)
	assert.EqualError(t, err, "invalid character 'b' looking for beginning of value")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func setupMockWebflowUserCreate(t *testing.T, testId string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", webflowCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
		lbResponse := leaderboardResponse{Id: testId}
		respBytes, err := json.Marshal(lbResponse)
		assert.NoError(t, err)
		_, _ = w.Write(respBytes)
	}))
	return ts
}

func TestUpstreamNewParticipantWebflowValidID(t *testing.T) {
	origWebflowColletion := webflowCollection
	defer func() {
		webflowCollection = origWebflowColletion
	}()
	webflowCollection = "testWfCollection"
	origWebflowToken := webflowToken
	defer func() {
		webflowToken = origWebflowToken
	}()
	webflowToken = "testWfToken"
	testId := "testNewWebflowParticipantId"
	ts := setupMockWebflowUserCreate(t, testId)
	defer ts.Close()
	origWebflowBaseAPI := webflowBaseAPI
	defer func() {
		webflowBaseAPI = origWebflowBaseAPI
	}()
	webflowBaseAPI = ts.URL

	c, rec := setupMockContextWebflow()

	id, err := upstreamNewParticipant(c, participant{})
	assert.Equal(t, testId, id)
	assert.NoError(t, err)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func setupMockContextParticipant(participantJson string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", strings.NewReader(participantJson))
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

func TestAddParticipantWebflowError(t *testing.T) {
	participantName := "partName"
	participantJson := `{"gitHubName": "` + participantName + `"}`
	c, rec := setupMockContextParticipant(participantJson)

	origWebflowToken := webflowToken
	defer func() {
		webflowToken = origWebflowToken
	}()
	webflowToken = "testWfToken"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", webflowCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	origWebflowBaseAPI := webflowBaseAPI
	defer func() {
		webflowBaseAPI = origWebflowBaseAPI
	}()
	webflowBaseAPI = ts.URL

	expectedErr := &ParticipantCreateError{"404 Not Found"}
	assert.EqualError(t, addParticipant(c), expectedErr.Error())
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, expectedErr.Error(), rec.Body.String())
}

func TestAddParticipantCampaignMissing(t *testing.T) {
	participantName := "partName"
	participantJson := `{"gitHubName": "` + participantName + `"}`
	c, rec := setupMockContextParticipant(participantJson)

	origWebflowToken := webflowToken
	defer func() {
		webflowToken = origWebflowToken
	}()
	webflowToken = "testWfToken"
	testId := "testNewWebflowParticipantId"
	ts := setupMockWebflowUserCreate(t, testId)
	defer ts.Close()
	origWebflowBaseAPI := webflowBaseAPI
	defer func() {
		webflowBaseAPI = origWebflowBaseAPI
	}()
	webflowBaseAPI = ts.URL

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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertParticipant)).
		WillReturnError(forcedError)

	assert.EqualError(t, addParticipant(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddParticipant(t *testing.T) {
	participantName := "partName"
	participantJson := `{"gitHubName": "` + participantName + `"}`
	c, rec := setupMockContextParticipant(participantJson)

	origWebflowToken := webflowToken
	defer func() {
		webflowToken = origWebflowToken
	}()
	webflowToken = "testWfToken"
	testId := "testNewWebflowParticipantId"
	ts := setupMockWebflowUserCreate(t, testId)
	defer ts.Close()
	origWebflowBaseAPI := webflowBaseAPI
	defer func() {
		webflowBaseAPI = origWebflowBaseAPI
	}()
	webflowBaseAPI = ts.URL

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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertParticipant)).
		WithArgs(participantName, "", "", 0, testId, "").
		WillReturnRows(sqlmock.NewRows([]string{"Id", "Score", "JoinedAt"}).AddRow(participantID, 0, time.Time{}))

	assert.NoError(t, addParticipant(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `{"guid":"`+participantID+`","endpoints":{"participantDetail"`), rec.Body.String())
	assert.True(t, strings.Contains(rec.Body.String(), `"gitHubName":"`+participantName+`"`), rec.Body.String())
}

func setupMockContextUpdateParticipant(participantJson string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", strings.NewReader(participantJson))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	return
}

func TestUpdateParticipantBodyInvalid(t *testing.T) {
	c, rec := setupMockContextUpdateParticipant("")

	assert.EqualError(t, updateParticipant(c), "EOF")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateParticipantMissingParticipantID(t *testing.T) {
	participantName := "partName"
	participantJson := `{"gitHubName": "` + participantName + `"}`
	c, rec := setupMockContextUpdateParticipant(participantJson)

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
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(participantName, "", "", 0, "", sql.NullString{}, "").
		WillReturnError(forcedError)

	assert.EqualError(t, updateParticipant(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateParticipantUpdateError(t *testing.T) {
	participantID := "participantUUId"
	participantName := "partName"
	participantJson := `{"guid": "` + participantID + `", "gitHubName": "` + participantName + `"}`
	c, rec := setupMockContextUpdateParticipant(participantJson)

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
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(participantName, "", "", 0, "", sql.NullString{}, participantID).
		WillReturnResult(sqlmock.NewErrorResult(forcedError))

	assert.EqualError(t, updateParticipant(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateParticipantScoreError(t *testing.T) {
	participantID := "participantUUId"
	participantName := "partName"
	participantJson := `{"guid": "` + participantID + `", "gitHubName": "` + participantName + `"}`
	c, rec := setupMockContextUpdateParticipant(participantJson)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(participantName, "", "", 0, "", sql.NullString{}, participantID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := updateParticipant(c)
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "all expectations were already fulfilled, call to Query 'UPDATE participants"))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func setupMockContextUpstreamUpdateScore() (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	return
}

func TestUpstreamUpdateScoreStatusError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/", webflowCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	origWebflowBaseAPI := webflowBaseAPI
	defer func() {
		webflowBaseAPI = origWebflowBaseAPI
	}()
	webflowBaseAPI = ts.URL

	origWebflowToken := webflowToken
	defer func() {
		webflowToken = origWebflowToken
	}()
	webflowToken = "testWfToken"

	c, rec := setupMockContextUpstreamUpdateScore()

	expectedErr := &ParticipantUpdateError{"404 Not Found"}
	assert.EqualError(t, upstreamUpdateScore(c, "", 0), expectedErr.Error())
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, expectedErr.Error(), rec.Body.String())
}

func setupMockWebflowUserUpdate(t *testing.T, webflowId string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", webflowCollection, webflowId), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
	}))
	return ts
}

func TestUpstreamUpdateScore(t *testing.T) {
	ts := setupMockWebflowUserUpdate(t, "")
	defer ts.Close()
	origWebflowBaseAPI := webflowBaseAPI
	defer func() {
		webflowBaseAPI = origWebflowBaseAPI
	}()
	webflowBaseAPI = ts.URL

	origWebflowToken := webflowToken
	defer func() {
		webflowToken = origWebflowToken
	}()
	webflowToken = "testWfToken"

	c, rec := setupMockContextUpstreamUpdateScore()

	assert.NoError(t, upstreamUpdateScore(c, "", 0))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateParticipantNoRowsUpdated(t *testing.T) {
	participantID := "participantUUId"
	participantName := "partName"
	participantJson := `{"guid": "` + participantID + `", "gitHubName": "` + participantName + `"}`
	c, rec := setupMockContextUpdateParticipant(participantJson)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(participantName, "", "", 0, "", sql.NullString{}, participantID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(0, participantName).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(participantID, 0))

	ts := setupMockWebflowUserUpdate(t, participantID)
	defer ts.Close()
	origWebflowBaseAPI := webflowBaseAPI
	defer func() {
		webflowBaseAPI = origWebflowBaseAPI
	}()
	webflowBaseAPI = ts.URL

	origWebflowToken := webflowToken
	defer func() {
		webflowToken = origWebflowToken
	}()
	webflowToken = "testWfToken"

	assert.NoError(t, updateParticipant(c))
	assert.Equal(t, http.StatusBadRequest, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateParticipant(t *testing.T) {
	participantID := "participantUUId"
	participantName := "partName"
	participantJson := `{"guid": "` + participantID + `", "gitHubName": "` + participantName + `"}`
	c, rec := setupMockContextUpdateParticipant(participantJson)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(participantName, "", "", 0, "", sql.NullString{}, participantID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(0, participantName).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(participantID, 0))

	ts := setupMockWebflowUserUpdate(t, participantID)
	defer ts.Close()
	origWebflowBaseAPI := webflowBaseAPI
	defer func() {
		webflowBaseAPI = origWebflowBaseAPI
	}()
	webflowBaseAPI = ts.URL

	origWebflowToken := webflowToken
	defer func() {
		webflowToken = origWebflowToken
	}()
	webflowToken = "testWfToken"

	assert.NoError(t, updateParticipant(c))
	assert.Equal(t, http.StatusNoContent, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func setupMockContextTeam(teamJson string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", strings.NewReader(teamJson))
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertTeam)).
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertTeam)).
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertTeam)).
		WithArgs(teamName, sql.NullString{String: organizationName, Valid: true}).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow(teamID))

	assert.NoError(t, addTeam(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, teamID, rec.Body.String())
}

func setupMockContextAddPersonToTeam(githubName, teamName string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames(ParamGithubName, ParamTeamName)
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
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipantTeam)).
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
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipantTeam)).
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

	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipantTeam)).
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

	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipantTeam)).
		WithArgs(teamName, githubName).
		WillReturnResult(sqlmock.NewResult(0, 5))

	assert.NoError(t, addPersonToTeam(c))
	assert.Equal(t, http.StatusNoContent, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func setupMockContextParticipantDetail(githubName string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames(ParamGithubName)
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantDetail)).
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantDetail)).
		WithArgs(githubName).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "GHName", "Email", "DisplayName", "Score", "TeamName", "JoinedAt", "CampaignName"}).AddRow(participantID, githubName, "", "", 0, "", time.Time{}, ""))

	assert.NoError(t, getParticipantDetail(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `{"guid":"`+participantID+`","gitHubName":"`+githubName+`"`), rec.Body.String())
}

func setupMockContextParticipantList(campaignName string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames(ParamCampaignName)
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantsByCampaign)).
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

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantsByCampaign)).
		WithArgs("").
		WillReturnRows(sqlmock.NewRows([]string{"Id", "GHName", "Email", "DisplayName", "Score", "TeamName", "JoinedAt", "CampaignName"}).
			// force scan error due to time.Time type mismatch at JoinedAt column
			AddRow(-1, "", "", "", 0, "", "", campaignName))

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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantsByCampaign)).
		WithArgs(campaignName).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "GHName", "Email", "DisplayName", "Score", "TeamName", "JoinedAt", "CampaignName"}).
			AddRow(participantID, "", "", "", 0, "", time.Time{}, campaignName))

	assert.NoError(t, getParticipantsList(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `[{"guid":"`+participantID+`","gitHubName":""`), rec.Body.String())
	assert.True(t, strings.HasSuffix(rec.Body.String(), `","campaignName":"`+campaignName+`"}]`+"\n"), rec.Body.String())
}

func setupMockContextAddBug(bugJson string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", strings.NewReader(bugJson))
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertBug)).
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertBug)).
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
	req := httptest.NewRequest("", "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames(ParamBugCategory, ParamPointValue)
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
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateBug)).
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
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateBug)).
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

	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateBug)).
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

	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateBug)).
		WithArgs(pointValue, category).
		WillReturnResult(sqlmock.NewResult(0, 5))

	assert.NoError(t, updateBug(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.Equal(t, "Success", rec.Body.String())
}

func setupMockContextGetBugs() (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", nil)
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectBug)).WillReturnError(forcedError)

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

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectBug)).
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectBug)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "Category", "PointValue"}).
			// force scan error due to time.Time type mismatch at PointValue column
			AddRow(bugId, category, strconv.Itoa(pointValue)))

	assert.NoError(t, getBugs(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.Equal(t, `[{"guid":"`+bugId+`","category":"`+category+`","pointValue":`+strconv.Itoa(pointValue)+`}]`+"\n", rec.Body.String())
}

func setupMockContextPutBugs(bugsJson string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", strings.NewReader(bugsJson))
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertBug)).
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertBug)).
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertBug)).
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
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertBug)).
		WithArgs("", 0).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).
			AddRow(bugId))

	bugId2 := "secondBugId"
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertBug)).
		WithArgs("", 0).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).
			AddRow(bugId2))
	mock.ExpectCommit()

	assert.NoError(t, putBugs(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, `{"guid":"`+bugId+`","endpoints":null,"object":[{"guid":"`+bugId+`","category":"","pointValue":0},{"guid":"`+bugId2+`","category":"","pointValue":0}]}`+"\n", rec.Body.String())
}

func setupMockContextParticipantDelete(githubName string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames(ParamGithubName)
	c.SetParamValues(githubName)
	return
}

func TestDeleteParticipant(t *testing.T) {
	githubName := "myGithubName"
	c, rec := setupMockContextParticipantDelete(githubName)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectExec(convertSqlToDbMockExpect(sqlDeleteParticipant)).
		WithArgs(githubName).
		WillReturnResult(sqlmock.NewResult(0, 1))

	assert.NoError(t, deleteParticipant(c))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestValidScoreUnknownOwner(t *testing.T) {
	c := setupMockContext()

	assert.False(t, validScore(c, "", ""))
}

func setupMockContext() echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c
}

const testOrgValid = "thanos-io"

func TestValidScoreParticipantNotRegistered(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	githubName := "myGithubName"
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(githubName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}))

	c := setupMockContext()

	assert.False(t, validScore(c, testOrgValid, "unregisteredUser"))
}

func TestValidScoreParticipant(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	githubName := "myGithubName"
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(githubName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow("someId"))

	assert.True(t, validScore(nil, testOrgValid, githubName))
}

func TestScorePointsNothing(t *testing.T) {
	msg := scoringMessage{}
	points, err := scorePoints(msg)
	assert.NoError(t, err)
	assert.Equal(t, 0, points)
}

func TestScorePointsScanError(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	msg := scoringMessage{BugCounts: map[string]int{"myBugType": 1}}
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectPointValue)).
		WithArgs("unexpectedBugType").
		WillReturnRows(sqlmock.NewRows([]string{"Value"}).AddRow(1))

	points, err := scorePoints(msg)
	assert.EqualError(t, err, "Query 'SELECT pointValue FROM bugs WHERE category = $1', arguments do not match: argument 0 expected [string - unexpectedBugType] does not match actual [string - myBugType]")
	assert.Equal(t, 1, points)
}

func TestScorePointsFixedTwoThreePointers(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	bugType := "threePointBugType"
	msg := scoringMessage{BugCounts: map[string]int{bugType: 2}}
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectPointValue)).
		WithArgs(bugType).
		WillReturnRows(sqlmock.NewRows([]string{"Value"}).AddRow(3))

	points, err := scorePoints(msg)
	assert.NoError(t, err)
	assert.Equal(t, 6, points)
}

func TestScorePointsBonusForNonClassified(t *testing.T) {
	msg := scoringMessage{TotalFixed: 1}
	points, err := scorePoints(msg)
	assert.NoError(t, err)
	assert.Equal(t, 1, points)
}

func setupMockContextNewScore(t *testing.T, alert scoringAlert) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	alertBytes, err := json.Marshal(alert)
	assert.NoError(t, err)
	alertJson := string(alertBytes)
	req := httptest.NewRequest(http.MethodPost, New, strings.NewReader(alertJson))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	return
}

func TestNewScoreMalformedAlert(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, New, strings.NewReader("notAnAlert"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := newScore(c)
	assert.EqualError(t, err, "invalid character 'o' in literal null (expecting 'u')")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreEmptyAlert(t *testing.T) {
	c, rec := setupMockContextNewScore(t, scoringAlert{})
	err := newScore(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertInvalidScoringMessage(t *testing.T) {
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{"badScoringMessage"},
	})
	err := newScore(c)
	assert.EqualError(t, err, "invalid character 'b' looking for beginning of value")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertInvalidScore(t *testing.T) {
	scoringMsgBytes, err := json.Marshal(scoringMessage{})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})
	err = newScore(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertInvalidScore_NoTriggerUserFound(t *testing.T) {
	githubName := "myGithubName"
	scoringMsgBytes, err := json.Marshal(scoringMessage{RepoOwner: testOrgValid, TriggerUser: githubName})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(githubName).
		WillReturnRows(sqlmock.NewRows([]string{}))

	err = newScore(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertScorePointsError(t *testing.T) {
	githubName := "myGithubName"
	scoringMsgBytes, err := json.Marshal(scoringMessage{RepoOwner: testOrgValid, TriggerUser: githubName, BugCounts: map[string]int{"myBugType": 1}})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(githubName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow("someId"))

	err = newScore(c)
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "all expectations were already fulfilled, call to Query 'SELECT pointValue FROM bugs"))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertHandleBeginTransactionError(t *testing.T) {
	githubName := "myGithubName"
	scoringMsgBytes, err := json.Marshal(scoringMessage{RepoOwner: testOrgValid, TriggerUser: githubName})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(githubName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow("someId"))

	err = newScore(c)
	assert.EqualError(t, err, "all expectations were already fulfilled, call to database transaction Begin was not expected")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertScoreQueryErrorIgnored(t *testing.T) {
	githubName := "myGithubName"
	scoringMsgBytes, err := json.Marshal(scoringMessage{RepoOwner: testOrgValid, TriggerUser: githubName})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(githubName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow("someId"))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WillReturnRows(sqlmock.NewRows([]string{}))

	err = newScore(c)
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "all expectations were already fulfilled, call to ExecQuery 'INSERT INTO scoring_events"))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertInsertScoringEventErrorNotIgnored(t *testing.T) {
	githubName := "myGithubName"
	githubRepoName := "myRepoName"
	githubPrId := -5
	scoringMsgBytes, err := json.Marshal(scoringMessage{RepoOwner: testOrgValid, TriggerUser: githubName, RepoName: githubRepoName, PullRequest: githubPrId})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(githubName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow("someId"))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(testOrgValid, githubRepoName, githubPrId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	err = newScore(c)
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "all expectations were already fulfilled, call to ExecQuery 'INSERT INTO scoring_events"))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertUpdateScoreErrorNotIgnored(t *testing.T) {
	githubName := "myGithubName"
	githubRepoName := "myRepoName"
	githubPrId := -5
	scoringMsgBytes, err := json.Marshal(scoringMessage{RepoOwner: testOrgValid, TriggerUser: githubName, RepoName: githubRepoName, PullRequest: githubPrId})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(githubName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow("someId"))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(testOrgValid, githubRepoName, githubPrId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	mock.ExpectExec(convertSqlToDbMockExpect(sqlInsertScoringEvent)).
		WithArgs(testOrgValid, githubRepoName, githubPrId, githubName, 0).
		WillReturnResult(sqlmock.NewResult(0, -1))

	err = newScore(c)
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "all expectations were already fulfilled, call to Query 'UPDATE participants"))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertUpdateScoreEndPointErrorNotIgnored(t *testing.T) {
	githubName := "myGithubName"
	githubRepoName := "myRepoName"
	githubPrId := -5
	scoringMsgBytes, err := json.Marshal(scoringMessage{RepoOwner: testOrgValid, TriggerUser: githubName, RepoName: githubRepoName, PullRequest: githubPrId})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(githubName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow("someId"))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(testOrgValid, githubRepoName, githubPrId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	mock.ExpectExec(convertSqlToDbMockExpect(sqlInsertScoringEvent)).
		WithArgs(testOrgValid, githubRepoName, githubPrId, githubName, 0).
		WillReturnResult(sqlmock.NewResult(0, -1))

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(8, githubName).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(githubName, 0))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", webflowCollection, githubName), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()
	origWebflowBaseAPI := webflowBaseAPI
	defer func() {
		webflowBaseAPI = origWebflowBaseAPI
	}()
	webflowBaseAPI = ts.URL

	origWebflowToken := webflowToken
	defer func() {
		webflowToken = origWebflowToken
	}()
	webflowToken = "testWfToken"

	err = newScore(c)
	assert.EqualError(t, err, "could not update score. response status: 400 Bad Request")
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, "could not update score. response status: 400 Bad Request", rec.Body.String())
}

func TestNewScoreOneAlertCommitErrorNotIgnored(t *testing.T) {
	githubName := "myGithubName"
	githubRepoName := "myRepoName"
	githubPrId := -5
	scoringMsgBytes, err := json.Marshal(scoringMessage{RepoOwner: testOrgValid, TriggerUser: githubName, RepoName: githubRepoName, PullRequest: githubPrId})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(githubName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow("someId"))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(testOrgValid, githubRepoName, githubPrId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	mock.ExpectExec(convertSqlToDbMockExpect(sqlInsertScoringEvent)).
		WithArgs(testOrgValid, githubRepoName, githubPrId, githubName, 0).
		WillReturnResult(sqlmock.NewResult(0, -1))

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(8, githubName).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(githubName, 0))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", webflowCollection, githubName), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	origWebflowBaseAPI := webflowBaseAPI
	defer func() {
		webflowBaseAPI = origWebflowBaseAPI
	}()
	webflowBaseAPI = ts.URL

	origWebflowToken := webflowToken
	defer func() {
		webflowToken = origWebflowToken
	}()
	webflowToken = "testWfToken"

	err = newScore(c)
	assert.EqualError(t, err, "all expectations were already fulfilled, call to Commit transaction was not expected")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlert(t *testing.T) {
	githubName := "myGithubName"
	githubRepoName := "myRepoName"
	githubPrId := -5
	scoringMsgBytes, err := json.Marshal(scoringMessage{RepoOwner: testOrgValid, TriggerUser: githubName, RepoName: githubRepoName, PullRequest: githubPrId})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(githubName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow("someId"))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(testOrgValid, githubRepoName, githubPrId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	mock.ExpectExec(convertSqlToDbMockExpect(sqlInsertScoringEvent)).
		WithArgs(testOrgValid, githubRepoName, githubPrId, githubName, 0).
		WillReturnResult(sqlmock.NewResult(0, -1))

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(8, githubName).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(githubName, 0))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", webflowCollection, githubName), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	origWebflowBaseAPI := webflowBaseAPI
	defer func() {
		webflowBaseAPI = origWebflowBaseAPI
	}()
	webflowBaseAPI = ts.URL

	origWebflowToken := webflowToken
	defer func() {
		webflowToken = origWebflowToken
	}()
	webflowToken = "testWfToken"

	mock.ExpectCommit()

	err = newScore(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}
