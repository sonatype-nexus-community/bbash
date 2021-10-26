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
	"html/template"
	"net"
	"net/http"
	"net/http/httptest"
	url2 "net/url"
	"os"
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

func resetEnvVar(t *testing.T, envVarName, origValue string) {
	if origValue != "" {
		assert.NoError(t, os.Setenv(envVarName, origValue))
	} else {
		assert.NoError(t, os.Unsetenv(envVarName))
	}
}

func resetEnvVarPGHost(t *testing.T, origEnvPGHost string) {
	resetEnvVar(t, envPGHost, origEnvPGHost)
}

func TestMainDBPingError(t *testing.T) {
	errRecovered = nil
	origEnvPGHost := os.Getenv(envPGHost)
	defer func() {
		resetEnvVarPGHost(t, origEnvPGHost)
	}()
	assert.NoError(t, os.Setenv(envPGHost, "bogus-db-hostname"))

	defer func() {
		errRecovered = nil
	}()

	main()

	assert.True(t, strings.HasPrefix(errRecovered.Error(), "failed to ping database. host: bogus-db-hostname, port: "))
}

func TestMainDBMigrateError(t *testing.T) {
	errRecovered = nil
	origEnvPGHost := os.Getenv(envPGHost)
	defer func() {
		resetEnvVarPGHost(t, origEnvPGHost)
	}()
	assert.NoError(t, os.Setenv(envPGHost, "localhost"))

	// setup mock db endpoint
	l, err := net.Listen("tcp", "localhost:0")
	assert.NoError(t, err)
	defer func(l net.Listener) {
		_ = l.Close()
	}(l)
	//goland:noinspection HttpUrlsUsage
	u, err := url2.Parse("http://" + l.Addr().String())
	assert.NoError(t, err)
	freeLocalPort := u.Port()
	assert.NoError(t, os.Setenv(envPGPort, freeLocalPort))
	go func() {
		conn, err := l.Accept()
		assert.NoError(t, err)
		defer func(conn net.Conn) {
			_ = conn.Close()
		}(conn)
		b := make([]byte, 0, 512)
		count, err := conn.Read(b)
		_, _ = conn.Write(b)
		assert.NoError(t, err)
		assert.Equal(t, count, 0)
	}()

	defer func() {
		errRecovered = nil
	}()

	main()

	assert.True(t, strings.HasPrefix(errRecovered.Error(), "failed to ping database. host: localhost, port: "+freeLocalPort), errRecovered.Error())
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

//goland:noinspection GoUnusedFunction,GoSnakeCaseUsage
func xxxIgnore_TestMigrateDB(t *testing.T) {
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

func TestGetCampaignsQueryError(t *testing.T) {
	c, rec := setupMockContext()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced campaign error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaign)).
		WillReturnError(forcedError)

	assert.EqualError(t, getCampaigns(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetCampaignsScanError(t *testing.T) {
	c, rec := setupMockContext()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaign)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "active", "note"}).
			// force scan error due to time.Time type mismatch at CreatedOn column
			AddRow("campaignId", campaign, "badness", 1, true, ""))

	assert.EqualError(t, getCampaigns(c), `sql: Scan error on column index 2, name "createdOn": unsupported Scan, storing driver.Value type string into type *time.Time`)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetCampaigns(t *testing.T) {
	c, rec := setupMockContext()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaign)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "active", "note"}).
			AddRow("campaignId", campaign, time.Time{}, 1, true, ""))

	assert.NoError(t, getCampaigns(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.Equal(t, "[{\"guid\":\"campaignId\",\"name\":\"myCampaignName\",\"createdOn\":\"0001-01-01T00:00:00Z\",\"createdOrder\":1,\"active\":true,\"note\":{\"String\":\"\",\"Valid\":true}}]\n", rec.Body.String())
}

func TestGetCurrentCampaignScanError(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCurrentCampaign)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "active", "upstreamId", "note"}).
			// force scan error due to time.Time type mismatch at CreatedOn column
			AddRow(campaignId, campaign, "badness", 0, true, "myUpstreamId", sql.NullString{}))

	currentCampaign, err := getCurrentCampaign()
	assert.EqualError(t, err, `sql: Scan error on column index 2, name "createdOn": unsupported Scan, storing driver.Value type string into type *time.Time`)
	assert.Equal(t, campaignStruct{ID: campaignId, Name: campaign}, currentCampaign)
}

func TestGetCurrentCampaign(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mockCurrentCampaign(mock)

	currentCampaign, err := getCurrentCampaign()
	assert.NoError(t, err)
	assert.Equal(t, campaignStruct{ID: campaignId, Name: campaign, Active: true}, currentCampaign)
}

func mockCurrentCampaign(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCurrentCampaign)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "active", "upstreamId", "note"}).
			AddRow(campaignId, campaign, time.Time{}, 0, true, "myUpstreamId", sql.NullString{}))
}

func TestGetCurrentCampaignEchoError(t *testing.T) {
	c, rec := setupMockContextCampaign(campaign)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced campaign error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCurrentCampaign)).
		WillReturnError(forcedError)

	assert.NoError(t, getCurrentCampaignEcho(c))
	assert.Equal(t, http.StatusBadRequest, c.Response().Status)
	assert.Equal(t, forcedError.Error(), rec.Body.String())
}

func TestGetCurrentCampaignEcho(t *testing.T) {
	c, rec := setupMockContextCampaign(campaign)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCurrentCampaign)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "active", "upstreamId", "note"}).
			AddRow(campaignId, campaign, time.Time{}, 0, true, "myUpstreamId", sql.NullString{}))

	assert.NoError(t, getCurrentCampaignEcho(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.Equal(t, "{\"guid\":\"myCampaignId\",\"name\":\"myCampaignName\",\"createdOn\":\"0001-01-01T00:00:00Z\",\"createdOrder\":0,\"active\":true,\"note\":{\"String\":\"\",\"Valid\":false}}\n", rec.Body.String())
}

func TestAddCampaignUpstreamAddError(t *testing.T) {
	origCampaignCollection := upstreamConfig.campaignCollection
	defer func() {
		upstreamConfig.campaignCollection = origCampaignCollection
	}()
	upstreamConfig.campaignCollection = "testWfCollection"
	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"
	testId := "testNewWebflowCampaignId"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", upstreamConfig.campaignCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusInternalServerError)
	}))

	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextCampaign(campaign)

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
		WithArgs(campaign, testId).
		WillReturnError(forcedError)

	expectedError := CreateError{msgPatternCreateErrorCampaign, "500 Internal Server Error"}
	assert.EqualError(t, addCampaign(c), expectedError.Error())
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, expectedError.Error(), rec.Body.String())
}

func TestAddCampaignScanError(t *testing.T) {
	origCampaignCollection := upstreamConfig.campaignCollection
	defer func() {
		upstreamConfig.campaignCollection = origCampaignCollection
	}()
	upstreamConfig.campaignCollection = "testWfCollection"
	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"
	testId := "testNewWebflowCampaignId"
	ts := setupMockWebflowCampaignCreate(t, testId)
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextCampaign(campaign)

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
		WithArgs(campaign, testId).
		WillReturnError(forcedError)

	assert.EqualError(t, addCampaign(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddCampaign(t *testing.T) {
	origCampaignCollection := upstreamConfig.campaignCollection
	defer func() {
		upstreamConfig.campaignCollection = origCampaignCollection
	}()
	upstreamConfig.campaignCollection = "testWfCollection"
	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"
	testId := "testNewWebflowCampaignId"
	ts := setupMockWebflowCampaignCreate(t, testId)
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextCampaign(campaign)

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
		WithArgs(campaign, testId).
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
	assert.Equal(t, fmt.Sprintf("Bearer %s", upstreamConfig.token), req.Header.Get("Authorization"))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Equal(t, "1.0.0", req.Header.Get("accept-version"))
}

func TestUpstreamNewCampaignWebflowErrorNotFound(t *testing.T) {
	origCampaignCollection := upstreamConfig.participantCollection
	defer func() {
		upstreamConfig.campaignCollection = origCampaignCollection
	}()
	upstreamConfig.campaignCollection = "testWfCampaignCollection"
	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", upstreamConfig.campaignCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextWebflow()

	id, err := upstreamNewCampaign(c, campaignStruct{})
	assert.Equal(t, "", id)
	expectedErr := &CreateError{msgPatternCreateErrorCampaign, "404 Not Found"}
	assert.EqualError(t, err, expectedErr.Error())
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, expectedErr.Error(), rec.Body.String())
}

func TestUpstreamNewCampaignWebflowIDDecodeError(t *testing.T) {
	origCampaignCollection := upstreamConfig.participantCollection
	defer func() {
		upstreamConfig.campaignCollection = origCampaignCollection
	}()
	upstreamConfig.campaignCollection = "testWfCollection"
	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", upstreamConfig.campaignCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("bad json text"))
	}))
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextWebflow()

	id, err := upstreamNewCampaign(c, campaignStruct{})
	assert.Equal(t, "", id)
	assert.EqualError(t, err, "invalid character 'b' looking for beginning of value")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func setupMockWebflowCampaignCreate(t *testing.T, testId string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", upstreamConfig.campaignCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
		lbResponse := leaderboardCampaignResponse{Id: testId}
		respBytes, err := json.Marshal(lbResponse)
		assert.NoError(t, err)
		tmpl, err := template.New("MockWebflowCampaignCreateResponse").Parse(string(respBytes))
		assert.NoError(t, err)
		err = tmpl.Execute(w, nil)
		assert.NoError(t, err)
	}))
	return ts
}

func TestUpstreamNewCampaignWebflowValidID(t *testing.T) {
	origCampaignCollection := upstreamConfig.campaignCollection
	defer func() {
		upstreamConfig.campaignCollection = origCampaignCollection
	}()
	upstreamConfig.campaignCollection = "testWfCollection"
	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"
	testId := "testNewWebflowCampaignId"
	ts := setupMockWebflowCampaignCreate(t, testId)
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextWebflow()

	id, err := upstreamNewCampaign(c, campaignStruct{})
	assert.Equal(t, testId, id)
	assert.NoError(t, err)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpstreamNewParticipantWebflowErrorNotFound(t *testing.T) {
	origParticipantCollection := upstreamConfig.participantCollection
	defer func() {
		upstreamConfig.participantCollection = origParticipantCollection
	}()
	upstreamConfig.participantCollection = "testWfCollection"
	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", upstreamConfig.participantCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextWebflow()

	id, err := upstreamNewParticipant(c, participant{})
	assert.Equal(t, "", id)
	expectedErr := &CreateError{msgPatternCreateErrorParticipant, "404 Not Found"}
	assert.EqualError(t, err, expectedErr.Error())
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, expectedErr.Error(), rec.Body.String())
}

func TestUpstreamNewParticipantWebflowIDDecodeError(t *testing.T) {
	origParticipantCollection := upstreamConfig.participantCollection
	defer func() {
		upstreamConfig.participantCollection = origParticipantCollection
	}()
	upstreamConfig.participantCollection = "testWfCollection"
	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", upstreamConfig.participantCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("bad json text"))
	}))
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

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
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", upstreamConfig.participantCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
		lbResponse := leaderboardResponse{Id: testId}
		respBytes, err := json.Marshal(lbResponse)
		assert.NoError(t, err)
		tmpl, err := template.New("MockWebflowUserCreateResponse").Parse(string(respBytes))
		assert.NoError(t, err)
		err = tmpl.Execute(w, nil)
		assert.NoError(t, err)
	}))
	return ts
}

func TestUpstreamNewParticipantWebflowValidID(t *testing.T) {
	origParticipantCollection := upstreamConfig.participantCollection
	defer func() {
		upstreamConfig.participantCollection = origParticipantCollection
	}()
	upstreamConfig.participantCollection = "testWfCollection"
	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"
	testId := "testNewWebflowParticipantId"
	ts := setupMockWebflowUserCreate(t, testId)
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

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
	participantJson := `{"loginName": "` + loginName + `"}`
	c, rec := setupMockContextParticipant(participantJson)

	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", upstreamConfig.participantCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	expectedErr := &CreateError{msgPatternCreateErrorParticipant, "404 Not Found"}
	assert.EqualError(t, addParticipant(c), expectedErr.Error())
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, expectedErr.Error(), rec.Body.String())
}

func TestAddParticipantCampaignMissing(t *testing.T) {
	participantJson := `{"loginName": "` + loginName + `"}`
	c, rec := setupMockContextParticipant(participantJson)

	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"
	testId := "testNewWebflowParticipantId"
	ts := setupMockWebflowUserCreate(t, testId)
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

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
	participantJson := fmt.Sprintf(`{"scpName": "%s","loginName": "%s"}`, scpName, loginName)
	c, rec := setupMockContextParticipant(participantJson)

	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"
	testId := "testNewWebflowParticipantId"
	ts := setupMockWebflowUserCreate(t, testId)
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

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
		WithArgs(scpName, "", loginName, "", "", 0, testId).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "Score", "JoinedAt"}).AddRow(participantID, 0, time.Time{}))

	assert.NoError(t, addParticipant(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `{"guid":"`+participantID+`","endpoints":{"participantDetail"`), rec.Body.String())
	assert.True(t, strings.Contains(rec.Body.String(), `"loginName":"`+loginName+`"`), rec.Body.String())
}

func TestMockWebflow_WithServer(t *testing.T) {
	participantJson := `{"loginName": "` + loginName + `"}`
	//c, rec := setupMockContextParticipant(participantJson)
	setupMockContextParticipant(participantJson)

	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"
	testId := "testNewWebflowParticipantId"
	ts := setupMockWebflowUserCreate(t, testId)
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	// uncomment 'main()' below for local testing with a mocked Webflow endpoint.
	//main()
}

//goland:noinspection GoSnakeCaseUsage,GoUnusedFunction
func xxxTestDropDB_DO_NOT_RUN_THIS(t *testing.T) {
	origEnvPGHost := os.Getenv(envPGHost)
	defer func() {
		resetEnvVarPGHost(t, origEnvPGHost)
	}()
	assert.NoError(t, os.Setenv(envPGHost, "localhost"))

	origEnvPGPort := os.Getenv(envPGPort)
	defer func() {
		resetEnvVarPGHost(t, origEnvPGPort)
	}()
	assert.NoError(t, os.Setenv(envPGPort, "5432"))

	origEnvPGUsername := os.Getenv(envPGUsername)
	defer func() {
		resetEnvVarPGHost(t, origEnvPGUsername)
	}()
	assert.NoError(t, os.Setenv(envPGUsername, "postgres"))

	origEnvPGPassword := os.Getenv(envPGPassword)
	defer func() {
		resetEnvVarPGHost(t, origEnvPGPassword)
	}()
	assert.NoError(t, os.Setenv(envPGPassword, "bug_bash"))

	origEnvPGDBName := os.Getenv(envPGDBName)
	defer func() {
		resetEnvVarPGHost(t, origEnvPGDBName)
	}()
	assert.NoError(t, os.Setenv(envPGDBName, "db"))

	origEnvSSLMode := os.Getenv(envSSLMode)
	defer func() {
		resetEnvVarPGHost(t, origEnvSSLMode)
	}()
	assert.NoError(t, os.Setenv(envSSLMode, "disable"))

	host, port, dbname, sslMode, err := openDB()
	defer func() {
		_ = db.Close()
	}()
	assert.NoError(t, err)
	assert.Equal(t, "localhost", host)
	assert.Equal(t, 5432, port)
	assert.Equal(t, "db", dbname)
	assert.Equal(t, "disable", sslMode)

	assert.NoError(t, migrateDB(db))
	assert.NoError(t, downgradeDB(db))
}

func TestLogAddParticipantWithError(t *testing.T) {
	c, rec := setupMockContext()
	err := logAddParticipant(c)
	assert.EqualError(t, err, "EOF")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestLogAddParticipantNoError(t *testing.T) {
	participantJson := fmt.Sprintf(`{"scpName": "%s","loginName": "%s"}`, scpName, loginName)
	c, rec := setupMockContextParticipant(participantJson)

	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"
	testId := "testNewWebflowParticipantId"
	ts := setupMockWebflowUserCreate(t, testId)
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

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
		WithArgs(scpName, "", loginName, "", "", 0, testId).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "Score", "JoinedAt"}).AddRow(participantID, 0, time.Time{}))

	err := logAddParticipant(c)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `{"guid":"`+participantID+`","endpoints":{"participantDetail"`), rec.Body.String())
	assert.True(t, strings.Contains(rec.Body.String(), `"loginName":"`+loginName+`"`), rec.Body.String())
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

// unit test values
const campaignId = "myCampaignId"
const campaign = "myCampaignName"
const scpName = "myScpName"
const participantID = "participantUUId"
const loginName = "loginName"
const teamName = "myTeamName"

func TestUpdateParticipantMissingParticipantID(t *testing.T) {
	participantJson := fmt.Sprintf(`{"loginName": "%s","campaignName": "%s", "scpName": "%s"}`, loginName, campaign, scpName)
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
		WithArgs(campaign, scpName, loginName, "", "", 0, sql.NullString{}, "").
		WillReturnError(forcedError)

	assert.EqualError(t, updateParticipant(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateParticipantUpdateError(t *testing.T) {
	participantJson := fmt.Sprintf(`{"guid": "%s","campaignName": "%s", "scpName": "%s", "loginName": "%s"}`, participantID, campaign, scpName, loginName)
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
		WithArgs(campaign, scpName, loginName, "", "", 0, sql.NullString{}, participantID).
		WillReturnResult(sqlmock.NewErrorResult(forcedError))

	assert.EqualError(t, updateParticipant(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateParticipantScoreError(t *testing.T) {
	participantJson := fmt.Sprintf(`{"guid": "%s","campaignName": "%s", "scpName": "%s", "loginName": "%s"}`, participantID, campaign, scpName, loginName)
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
		WithArgs(campaign, scpName, loginName, "", "", 0, sql.NullString{}, participantID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := updateParticipant(c)
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "all expectations were already fulfilled, call to Query 'UPDATE participant"))
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
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/", upstreamConfig.participantCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"

	c, rec := setupMockContextUpstreamUpdateScore()

	expectedErr := &ParticipantUpdateError{"404 Not Found"}
	assert.EqualError(t, upstreamUpdateScore(c, "", 0), expectedErr.Error())
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, expectedErr.Error(), rec.Body.String())
}

func setupMockWebflowUserUpdate(t *testing.T, webflowId string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", upstreamConfig.participantCollection, webflowId), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
	}))
	return ts
}

func TestUpstreamUpdateScore(t *testing.T) {
	ts := setupMockWebflowUserUpdate(t, "")
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"

	c, rec := setupMockContextUpstreamUpdateScore()

	assert.NoError(t, upstreamUpdateScore(c, "", 0))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateParticipantNoRowsUpdated(t *testing.T) {
	participantJson := fmt.Sprintf(`{"guid": "%s", "campaignName": "%s", "scpName": "%s", "loginName": "%s"}`, participantID, campaign, scpName, loginName)
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
		WithArgs(campaign, scpName, loginName, "", "", 0, sql.NullString{}, participantID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(0, campaign, scpName, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(participantID, 0))

	ts := setupMockWebflowUserUpdate(t, participantID)
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"

	assert.NoError(t, updateParticipant(c))
	assert.Equal(t, http.StatusBadRequest, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateParticipant(t *testing.T) {
	participantJson := fmt.Sprintf(`{"guid": "%s", "campaignName": "%s", "scpName": "%s", "loginName": "%s"}`, participantID, campaign, scpName, loginName)
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
		WithArgs(campaign, scpName, loginName, "", "", 0, sql.NullString{}, participantID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(0, campaign, scpName, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(participantID, 0))

	ts := setupMockWebflowUserUpdate(t, participantID)
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"

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
	teamJson := `{"campaign": "` + campaign + `","teamName": "` + teamName + `"}`
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
		WithArgs(campaign, teamName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow(teamID))

	assert.NoError(t, addTeam(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, teamID, rec.Body.String())
}

func TestAddTeam(t *testing.T) {
	teamJson := `{"campaign": "` + campaign + `","teamName":"` + teamName + `"}`
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
		WithArgs(campaign, teamName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow(teamID))

	assert.NoError(t, addTeam(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, teamID, rec.Body.String())
}

func setupMockContextAddPersonToTeam(campaignName, scpName, loginName, teamName string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames(ParamCampaignName, ParamScpName, ParamLoginName, ParamTeamName)
	c.SetParamValues(campaignName, scpName, loginName, teamName)
	return
}

func TestAddPersonToTeamMissingParameters(t *testing.T) {
	c, rec := setupMockContextAddPersonToTeam("", "", "", "")

	assert.NoError(t, addPersonToTeam(c))
	assert.Equal(t, http.StatusBadRequest, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddPersonToTeamUpdateError(t *testing.T) {
	c, rec := setupMockContextAddPersonToTeam(campaign, scpName, loginName, teamName)

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
	c, rec := setupMockContextAddPersonToTeam(campaign, scpName, loginName, teamName)

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
		WithArgs(teamName, campaign, scpName, loginName).
		WillReturnResult(sqlmock.NewErrorResult(forcedError))

	assert.EqualError(t, addPersonToTeam(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddPersonToTeamZeroRowsAffected(t *testing.T) {
	c, rec := setupMockContextAddPersonToTeam(campaign, scpName, loginName, teamName)

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
		WithArgs(teamName, campaign, scpName, loginName).
		WillReturnResult(sqlmock.NewResult(0, 0))

	assert.NoError(t, addPersonToTeam(c))
	assert.Equal(t, http.StatusBadRequest, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddPersonToTeamSomeRowsAffected(t *testing.T) {
	c, rec := setupMockContextAddPersonToTeam(campaign, scpName, loginName, teamName)

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
		WithArgs(teamName, campaign, scpName, loginName).
		WillReturnResult(sqlmock.NewResult(0, 5))

	assert.NoError(t, addPersonToTeam(c))
	assert.Equal(t, http.StatusNoContent, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func setupMockContextParticipantDetail(campaignName, scpName, loginName string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames(ParamCampaignName, ParamScpName, ParamLoginName)
	c.SetParamValues(campaignName, scpName, loginName)
	return
}

func TestGetParticipantDetailScanError(t *testing.T) {
	c, rec := setupMockContextParticipantDetail("", "", "")

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
		WithArgs("", "", "").
		WillReturnError(forcedError)

	assert.EqualError(t, getParticipantDetail(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetParticipantDetail(t *testing.T) {
	c, rec := setupMockContextParticipantDetail(campaign, scpName, loginName)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantDetail)).
		WithArgs(campaign, scpName, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignWFID", "SCPName", "LoginName", "Email", "DisplayName", "Score", "TeamName", "JoinedAt"}).AddRow(participantID, campaign, scpName, loginName, "", "", 0, "", time.Time{}))

	assert.NoError(t, getParticipantDetail(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `{"guid":"`+participantID+`","campaignName":"`+campaign+`","scpName":"`+scpName+`","loginName":"`+loginName+`"`), rec.Body.String())
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
		WillReturnRows(sqlmock.NewRows([]string{"Id", "Campaign", "SCPName", "LoginName", "Email", "DisplayName", "Score", "TeamName", "JoinedAt"}).
			// force scan error due to time.Time type mismatch at JoinedAt column
			AddRow(-1, campaignName, "", "", "", "", 0, "", ""))

	assert.EqualError(t, getParticipantsList(c), `sql: Scan error on column index 8, name "JoinedAt": unsupported Scan, storing driver.Value type string into type *time.Time`)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetParticipantsList(t *testing.T) {
	c, rec := setupMockContextParticipantList(campaign)

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
		WithArgs(campaign).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignWFID", "SCPName", "LoginName", "Email", "DisplayName", "Score", "TeamName", "JoinedAt"}).
			AddRow(participantID, campaign, "", "", "", "", 0, "", time.Time{}))

	assert.NoError(t, getParticipantsList(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `[{"guid":"`+participantID+`","campaignName":"`+campaign+`","scpName":"","loginName":""`), rec.Body.String())
}

func TestValidateBug(t *testing.T) {
	c, _ := setupMockContext()
	assert.EqualError(t, validateBug(c, bug{}), "bug is not valid, empty campaign: bug: {Id: Campaign: Category: PointValue:0}")
	assert.EqualError(t, validateBug(c, bug{Campaign: "myCampaign"}), "bug is not valid, empty category: bug: {Id: Campaign:myCampaign Category: PointValue:0}")
	assert.EqualError(t, validateBug(c, bug{Campaign: "myCampaign", Category: ""}), "bug is not valid, empty category: bug: {Id: Campaign:myCampaign Category: PointValue:0}")
	assert.EqualError(t, validateBug(c, bug{Campaign: "myCampaign", Category: "myCategory", PointValue: -1}), "bug is not valid, negative PointValue: bug: {Id: Campaign:myCampaign Category:myCategory PointValue:-1}")
	assert.NoError(t, validateBug(c, bug{Campaign: "myCampaign", Category: "myCategory", PointValue: 0}))
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
	campaign := "myCampaign"
	category := "myCategory"
	c, rec := setupMockContextAddBug(`{"campaign": "` + campaign + `", "category":"` + category + `"}`)

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
		WithArgs(campaign, category, 0).
		WillReturnError(forcedError)

	assert.EqualError(t, addBug(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddBugInvalidBug(t *testing.T) {
	c, rec := setupMockContextAddBug(`{}`)

	dbMock, _ := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	assert.EqualError(t, addBug(c), "bug is not valid, empty campaign: bug: {Id: Campaign: Category: PointValue:0}")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}
func TestAddBug(t *testing.T) {
	campaign := "myCampaign"
	category := "myCategory"
	pointValue := 9
	c, rec := setupMockContextAddBug(`{"campaign": "` + campaign + `", "category":"` + category + `","pointValue":` + strconv.Itoa(pointValue) + `}`)

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
		WithArgs(campaign, category, pointValue).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).
			AddRow(bugId))

	assert.NoError(t, addBug(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `{"guid":"`+bugId+`","endpoints":`), rec.Body.String())
	assert.True(t, strings.HasSuffix(rec.Body.String(), `"object":{"guid":"`+bugId+`","campaign":"`+campaign+`","category":"`+category+`","pointValue":`+strconv.Itoa(pointValue)+`}}`+"\n"), rec.Body.String())
}

func setupMockContextUpdateBug(campaign, bugCategory, pointValue string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames(ParamCampaignName, ParamBugCategory, ParamPointValue)
	c.SetParamValues(campaign, bugCategory, pointValue)
	return
}

func TestUpdateBugInvalidPointValue(t *testing.T) {
	c, rec := setupMockContextUpdateBug("", "", "non-number")

	assert.EqualError(t, updateBug(c), `strconv.Atoi: parsing "non-number": invalid syntax`)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateBugUpdateError(t *testing.T) {
	campaign := "myCampaign"
	category := "myCategory"
	pointValue := 9
	c, rec := setupMockContextUpdateBug(campaign, category, strconv.Itoa(pointValue))

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
		WithArgs(pointValue, campaign, category).
		WillReturnError(forcedError)

	assert.EqualError(t, updateBug(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateBugRowsAffectedError(t *testing.T) {
	campaign := "myCampaign"
	category := "myCategory"
	pointValue := 9
	c, rec := setupMockContextUpdateBug(campaign, category, strconv.Itoa(pointValue))

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
		WithArgs(pointValue, campaign, category).
		WillReturnResult(sqlmock.NewErrorResult(forcedError))

	assert.EqualError(t, updateBug(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateBugRowsAffectedZero(t *testing.T) {
	campaign := "myCampaign"
	category := "myCategory"
	pointValue := 9
	c, rec := setupMockContextUpdateBug(campaign, category, strconv.Itoa(pointValue))

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
		WithArgs(pointValue, campaign, category).
		WillReturnResult(sqlmock.NewResult(0, 0))

	assert.NoError(t, updateBug(c))
	assert.Equal(t, http.StatusNotFound, c.Response().Status)
	assert.Equal(t, "Bug Category not found", rec.Body.String())
}

func TestUpdateBugInvalidBug(t *testing.T) {
	c, rec := setupMockContextUpdateBug("myCampaign", "myCategory", "-1")

	dbMock, _ := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	assert.EqualError(t, updateBug(c), "bug is not valid, negative PointValue: bug: {Id: Campaign:myCampaign Category:myCategory PointValue:-1}")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateBug(t *testing.T) {
	campaign := "myCampaign"
	category := "myCategory"
	pointValue := 9
	c, rec := setupMockContextUpdateBug(campaign, category, strconv.Itoa(pointValue))

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
		WithArgs(pointValue, campaign, category).
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
		WillReturnRows(sqlmock.NewRows([]string{"Id", "Campaign", "Category", "PointValue"}).
			// force scan error due to time.Time type mismatch at PointValue column
			AddRow(-1, "", "", "non-number"))

	assert.EqualError(t, getBugs(c), `sql: Scan error on column index 3, name "PointValue": converting driver.Value type string ("non-number") to a int: invalid syntax`)
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

	campaign := "myCampaign"
	bugId := "myBugId"
	category := "myCategory"
	pointValue := 9
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectBug)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "Campaign", "Category", "PointValue"}).
			// force scan error due to time.Time type mismatch at PointValue column
			AddRow(bugId, campaign, category, strconv.Itoa(pointValue)))

	assert.NoError(t, getBugs(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.Equal(t, `[{"guid":"`+bugId+`","campaign":"`+campaign+`","category":"`+category+`","pointValue":`+strconv.Itoa(pointValue)+`}]`+"\n", rec.Body.String())
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
	c, rec := setupMockContextPutBugs(`[{"campaign":"myCampaign","category":"bugCat2", "pointvalue":5}]`)

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
		WithArgs("myCampaign", "bugCat2", 5).
		WillReturnError(forcedError)

	assert.EqualError(t, putBugs(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestPutBugsCommitTxError(t *testing.T) {
	c, rec := setupMockContextPutBugs(`[{"campaign":"myCampaign","category":"bugCat2", "pointvalue":5}]`)

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
		WithArgs("myCampaign", "bugCat2", 5).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).
			AddRow(""))
	forcedError := fmt.Errorf("forced Commit Txn error")
	mock.ExpectCommit().WillReturnError(forcedError)

	assert.EqualError(t, putBugs(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestPutBugsOneBugInvalidBug(t *testing.T) {
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

	assert.EqualError(t, putBugs(c), "bug is not valid, empty campaign: bug: {Id: Campaign: Category: PointValue:0}")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}
func TestPutBugsOneBug(t *testing.T) {
	c, rec := setupMockContextPutBugs(`[{"campaign":"myCampaign","category":"bugCat2", "pointvalue":5}]`)

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
		WithArgs("myCampaign", "bugCat2", 5).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).
			AddRow(bugId))
	mock.ExpectCommit()

	assert.NoError(t, putBugs(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, `{"guid":"`+bugId+`","endpoints":null,"object":[{"guid":"`+bugId+`","campaign":"myCampaign","category":"bugCat2","pointValue":5}]}`+"\n", rec.Body.String())
}

func TestPutBugsMultipleBugs(t *testing.T) {
	c, rec := setupMockContextPutBugs(`[{"campaign":"myCampaign","category":"bugCat2", "pointvalue":5}, {"campaign":"myCampaign","category":"bugCat3", "pointvalue":9}]`)

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
		WithArgs("myCampaign", "bugCat2", 5).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).
			AddRow(bugId))

	bugId2 := "secondBugId"
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertBug)).
		WithArgs("myCampaign", "bugCat3", 9).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).
			AddRow(bugId2))
	mock.ExpectCommit()

	assert.NoError(t, putBugs(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, `{"guid":"`+bugId+`","endpoints":null,"object":[{"guid":"`+bugId+`","campaign":"myCampaign","category":"bugCat2","pointValue":5},{"guid":"`+bugId2+`","campaign":"myCampaign","category":"bugCat3","pointValue":9}]}`+"\n", rec.Body.String())
}

func setupMockContextParticipantDelete(scpName, loginName string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames(ParamScpName, ParamLoginName)
	c.SetParamValues(scpName, loginName)
	return
}

func TestDeleteParticipant(t *testing.T) {
	scpName := "myScpName"
	loginName := "myLoginName"
	c, rec := setupMockContextParticipantDelete(scpName, loginName)

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
		WithArgs(scpName, loginName).
		WillReturnResult(sqlmock.NewResult(0, 1))

	assert.NoError(t, deleteParticipant(c))
	assert.Equal(t, http.StatusNoContent, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestDeleteParticipantWithRowsAffectedError(t *testing.T) {
	scpName := "myScpName"
	loginName := "myLoginName"
	c, rec := setupMockContextParticipantDelete(scpName, loginName)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced rows affected error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlDeleteParticipant)).
		WithArgs(scpName, loginName).
		WillReturnResult(sqlmock.NewErrorResult(forcedError))

	assert.NoError(t, deleteParticipant(c)) // we want to ignore this error case
	assert.Equal(t, http.StatusNotFound, c.Response().Status)
	assert.Equal(t, "\"no participant: scpName: myScpName, loginName: myLoginName\"\n", rec.Body.String())
}

func TestDeleteParticipantWithDeleteError(t *testing.T) {
	scpName := "myScpName"
	loginName := "myLoginName"
	c, rec := setupMockContextParticipantDelete(scpName, loginName)

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced delete error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlDeleteParticipant)).
		WithArgs(scpName, loginName).
		WillReturnError(forcedError)

	assert.EqualError(t, deleteParticipant(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestValidScoreUnknownOwner(t *testing.T) {
	c, _ := setupMockContext()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	orgName := "myOrgName"
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs(orgName).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	msg := scoringMessage{RepoOwner: orgName}
	isValid, campaignCurrent, scpNameOld := validScore(c, msg)
	assert.False(t, isValid)
	assert.Equal(t, campaignCurrent.Name, "")
	assert.Equal(t, scpNameOld, "")
}

func setupMockContext() (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	return
}

func setupMockContextWithBody(method string, body string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	return
}

// EventSource is lower case to match case sent by loggly
const testEventSourceValid = "github"
const testOrgValid = "myValidTestOrganization"

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

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(loginName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}))

	c, _ := setupMockContext()

	msg := scoringMessage{RepoOwner: testOrgValid, TriggerUser: "unregisteredUser"}
	isValid, campaignCurrent, scpNameOld := validScore(c, msg)
	assert.False(t, isValid)
	assert.Equal(t, campaignCurrent.Name, "")
	assert.Equal(t, scpNameOld, "")
}

func TestValidScoreParticipantCampaignError(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	setupMockDBOrgValid(mock)

	// current campaign error
	forcedError := fmt.Errorf("forced current campaign read error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCurrentCampaign)).
		WillReturnError(forcedError)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(campaign, testEventSourceValid, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "SCPName"}).AddRow("someId", "someSCP"))

	c, _ := setupMockContext()

	msg := scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName}
	isValid, campaignCurrent, scpNameDB := validScore(c, msg)
	assert.False(t, isValid)
	assert.Equal(t, campaignCurrent.Name, campaignCurrent.Name)
	assert.Equal(t, scpNameDB, "")
}

func TestValidScoreParticipantErrorReadingParticipant(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	setupMockDBOrgValid(mock)

	mockCurrentCampaign(mock)

	forcedError := fmt.Errorf("forced current campaign read error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(campaign, testEventSourceValid, loginName).
		WillReturnError(forcedError)

	c, _ := setupMockContext()

	msg := scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName}
	isValid, campaignCurrent, scpNameDB := validScore(c, msg)
	assert.False(t, isValid)
	assert.Equal(t, campaignCurrent.Name, campaignCurrent.Name)
	assert.Equal(t, scpNameDB, "")
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

	setupMockDBOrgValid(mock)

	mockCurrentCampaign(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(campaign, testEventSourceValid, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "SCPName"}).AddRow("someId", "someSCP"))

	c, _ := setupMockContext()

	msg := scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName}
	isValid, campaignCurrent, scpNameDB := validScore(c, msg)
	assert.True(t, isValid)
	assert.Equal(t, campaignCurrent.Name, campaignCurrent.Name)
	assert.Equal(t, scpNameDB, "someSCP")
}

func setupMockDBOrgValid(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs(testEventSourceValid, testOrgValid).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
}

func TestScorePointsNothing(t *testing.T) {
	msg := scoringMessage{}
	points, campaignName := scorePoints(nil, msg)
	assert.Equal(t, 0, points)
	assert.Equal(t, "", campaignName)
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

	c, _ := setupMockContext()

	points, campaignName := scorePoints(c, msg)
	assert.Equal(t, 1, points)
	assert.Equal(t, "", campaignName)
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
		WillReturnRows(sqlmock.NewRows([]string{"Value", "Campaign"}).AddRow(3, "someCampaign"))

	points, campaignName := scorePoints(nil, msg)
	assert.Equal(t, 6, points)
	assert.Equal(t, "someCampaign", campaignName)
}

func TestScorePointsBonusForNonClassified(t *testing.T) {
	msg := scoringMessage{TotalFixed: 1}
	points, campaignName := scorePoints(nil, msg)
	assert.Equal(t, 1, points)
	assert.Equal(t, "", campaignName)
}

func TestValidOrganizationFalse(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	orgName := "myOrgName"
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs(testEventSourceValid, orgName).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	msg := scoringMessage{EventSource: testEventSourceValid, RepoOwner: orgName}
	assert.False(t, validOrganization(nil, msg))
}

func TestValidOrganizationError(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	orgName := "myOrgName"
	forcedError := fmt.Errorf("forced query error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs(orgName).
		WillReturnError(forcedError)

	c, rec := setupMockContext()
	msg := scoringMessage{EventSource: "GitHub", RepoOwner: orgName}
	assert.False(t, validOrganization(c, msg))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestValidOrganization(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	orgName := "myOrgName"
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs(testEventSourceValid, orgName).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	msg := scoringMessage{EventSource: testEventSourceValid, RepoOwner: orgName}
	assert.True(t, validOrganization(nil, msg))
}

func TestLogNewScoreWithError(t *testing.T) {
	c, rec := setupMockContext()
	err := logNewScore(c)
	assert.EqualError(t, err, "EOF")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestLogNewScoreNoError(t *testing.T) {
	c, rec := setupMockContextNewScore(t, scoringAlert{})
	err := logNewScore(c)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusAccepted, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
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

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	orgName := ""
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs(orgName).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

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

func TestNewScoreOneAlertScorePointsMissingCampaignName(t *testing.T) {
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName, BugCounts: map[string]int{"myBugType": 1}})
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

	setupMockDBOrgValid(mock)

	// empty CampaignWFID is intentionally bad here
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCurrentCampaign)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "active", "upstreamId", "note"}).
			AddRow(campaignId, "", time.Time{}, 0, false, "myUpstreamId", sql.NullString{}))

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs("", testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "SCPName"}).AddRow("someId", scpName))

	err = newScore(c)
	assert.EqualError(t, err, fmt.Sprintf("empty current campaign name. campaign: %+v", campaignStruct{ID: campaignId}))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertScorePointsMissingSCPName(t *testing.T) {
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName, BugCounts: map[string]int{"myBugType": 1}})
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

	setupMockDBOrgValid(mock)

	mockCurrentCampaign(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(campaign, testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "SCPName"}).AddRow("someId", ""))

	err = newScore(c)
	assert.EqualError(t, err, fmt.Sprintf("empty db scpName. campaign: %+v", campaignStruct{ID: campaignId, Name: campaign, Active: true}))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertScorePointsMissingPointValue(t *testing.T) {
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName, BugCounts: map[string]int{"myBugType": 1}})
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

	setupMockDBOrgValid(mock)

	mockCurrentCampaign(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(campaign, testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "SCPName"}).AddRow("someId", scpName))

	err = newScore(c)
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "all expectations were already fulfilled, call to database transaction Begin was not expected"))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertHandleBeginTransactionError(t *testing.T) {
	loginName := "myLoginName"
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName})
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

	setupMockDBOrgValid(mock)

	mockCurrentCampaign(mock)

	scpName := "GitHub"
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(campaign, testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "SCPName"}).AddRow("someId", scpName))

	err = newScore(c)
	assert.EqualError(t, err, "all expectations were already fulfilled, call to database transaction Begin was not expected")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertScoreQueryErrorIgnored(t *testing.T) {
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName})
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

	setupMockDBOrgValid(mock)

	mockCurrentCampaign(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(campaign, testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "SCPName"}).AddRow("someId", scpName))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WillReturnRows(sqlmock.NewRows([]string{}))

	err = newScore(c)
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "all expectations were already fulfilled, call to ExecQuery 'INSERT INTO scoring_event"))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertInsertScoringEventErrorNotIgnored(t *testing.T) {
	githubRepoName := "myRepoName"
	githubPrId := -5
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName, RepoName: githubRepoName, PullRequest: githubPrId})
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

	setupMockDBOrgValid(mock)

	mockCurrentCampaign(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(campaign, testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "SCPName"}).AddRow("someId", scpName))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(campaign, scpName, testOrgValid, githubRepoName, githubPrId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	err = newScore(c)
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "all expectations were already fulfilled, call to ExecQuery 'INSERT INTO scoring_event"))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertUpdateScoreErrorNotIgnored(t *testing.T) {
	githubRepoName := "myRepoName"
	githubPrId := -5
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName, RepoName: githubRepoName, PullRequest: githubPrId})
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

	setupMockDBOrgValid(mock)

	mockCurrentCampaign(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(campaign, testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "SCPName"}).AddRow("someId", scpName))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(campaign, scpName, testOrgValid, githubRepoName, githubPrId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	mock.ExpectExec(convertSqlToDbMockExpect(sqlInsertScoringEvent)).
		WithArgs(campaign, scpName, testOrgValid, githubRepoName, githubPrId, strings.ToLower(loginName), 0).
		WillReturnResult(sqlmock.NewResult(0, -1))

	err = newScore(c)
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "all expectations were already fulfilled, call to Query 'UPDATE participant"))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertUpdateScoreEndPointErrorNotIgnored(t *testing.T) {
	githubRepoName := "myRepoName"
	githubPrId := -5
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName, RepoName: githubRepoName, PullRequest: githubPrId})
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

	setupMockDBOrgValid(mock)

	mockCurrentCampaign(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(campaign, testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "SCPName"}).AddRow("someId", scpName))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(campaign, scpName, testOrgValid, githubRepoName, githubPrId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	mock.ExpectExec(convertSqlToDbMockExpect(sqlInsertScoringEvent)).
		WithArgs(campaign, scpName, testOrgValid, githubRepoName, githubPrId, strings.ToLower(loginName), 0).
		WillReturnResult(sqlmock.NewResult(0, -1))

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(8, campaign, scpName, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(strings.ToLower(loginName), 0))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", upstreamConfig.participantCollection, strings.ToLower(loginName)), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"

	err = newScore(c)
	assert.EqualError(t, err, "could not update score. response status: 400 Bad Request")
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, "could not update score. response status: 400 Bad Request", rec.Body.String())
}

func TestNewScoreOneAlertCommitErrorNotIgnored(t *testing.T) {
	githubRepoName := "myRepoName"
	githubPrId := -5
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName, RepoName: githubRepoName, PullRequest: githubPrId})
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

	setupMockDBOrgValid(mock)

	mockCurrentCampaign(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(campaign, testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "SCPName"}).AddRow("someId", scpName))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(campaign, scpName, testOrgValid, githubRepoName, githubPrId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	mock.ExpectExec(convertSqlToDbMockExpect(sqlInsertScoringEvent)).
		WithArgs(campaign, scpName, testOrgValid, githubRepoName, githubPrId, strings.ToLower(loginName), 0).
		WillReturnResult(sqlmock.NewResult(0, -1))

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(8, campaign, scpName, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(strings.ToLower(loginName), 0))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", upstreamConfig.participantCollection, strings.ToLower(loginName)), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"

	err = newScore(c)
	assert.EqualError(t, err, "all expectations were already fulfilled, call to Commit transaction was not expected")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertUserCapitalizationMismatch(t *testing.T) {
	githubName := "MYGithubName"
	githubNameLowerCase := strings.ToLower(githubName)
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

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs(testOrgValid).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(githubNameLowerCase).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow("someId"))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(testOrgValid, githubRepoName, githubPrId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	mock.ExpectExec(convertSqlToDbMockExpect(sqlInsertScoringEvent)).
		WithArgs(testOrgValid, githubRepoName, githubPrId, githubNameLowerCase, 0).
		WillReturnResult(sqlmock.NewResult(0, -1))

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(8, githubNameLowerCase).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(githubNameLowerCase, 0))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", upstreamConfig.participantCollection, githubNameLowerCase), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"

	mock.ExpectCommit()

	err = newScore(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, c.Response().Status)
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
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", upstreamConfig.participantCollection, githubName), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	origBaseAPI := upstreamConfig.baseAPI
	defer func() {
		upstreamConfig.baseAPI = origBaseAPI
	}()
	upstreamConfig.baseAPI = ts.URL

	origToken := upstreamConfig.token
	defer func() {
		upstreamConfig.token = origToken
	}()
	upstreamConfig.token = "testWfToken"

	mock.ExpectCommit()

	err = newScore(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetSourceControlProvidersQueryError(t *testing.T) {
	c, rec := setupMockContext()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced scp error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectSourceControlProvider)).
		WillReturnError(forcedError)

	err := getSourceControlProviders(c)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetSourceControlProvidersScanError(t *testing.T) {
	c, rec := setupMockContext()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectSourceControlProvider)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "name", "url"}).
			// force scan error via invalid datatype
			AddRow("someId", "someSCP", sql.NullString{}))

	err := getSourceControlProviders(c)
	assert.EqualError(t, err, "sql: Scan error on column index 2, name \"url\": converting NULL to string is unsupported")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetSourceControlProviders(t *testing.T) {
	c, rec := setupMockContext()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectSourceControlProvider)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "name", "url"}).AddRow("someId", "someSCP", "someUrl"))

	err := getSourceControlProviders(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.Equal(t, "[{\"guid\":\"someId\",\"scpName\":\"someSCP\",\"url\":\"someUrl\"}]\n", rec.Body.String())
}

func TestGetOrganizationsError(t *testing.T) {
	c, rec := setupMockContext()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedErr := fmt.Errorf("forced org list error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganization)).
		WillReturnError(forcedErr)

	err := getOrganizations(c)
	assert.EqualError(t, err, forcedErr.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetOrganizationsScanError(t *testing.T) {
	c, rec := setupMockContext()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganization)).
		WillReturnRows(sqlmock.NewRows([]string{}).AddRow())

	err := getOrganizations(c)
	assert.EqualError(t, err, "sql: expected 0 destination arguments in Scan, not 3")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetOrganizations(t *testing.T) {
	c, rec := setupMockContext()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganization)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "SCPName", "Org"}).AddRow("someId", "someSCP", "someOrg"))

	err := getOrganizations(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.Equal(t, "[{\"guid\":\"someId\",\"scpName\":\"someSCP\",\"organization\":\"someOrg\"}]\n", rec.Body.String())
}

func TestAddOrganizationBodyBad(t *testing.T) {
	c, rec := setupMockContext()

	err := addOrganization(c)
	assert.EqualError(t, err, "EOF")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddOrganizationInsertError(t *testing.T) {
	c, rec := setupMockContextWithBody(http.MethodPut, "{\"organization\":\"myOrganizationName\"}")

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced org add error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlAddOrganization)).
		WillReturnError(forcedError)

	err := addOrganization(c)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddOrganization(t *testing.T) {
	c, rec := setupMockContextWithBody(http.MethodPut, "{\"organization\":\"myOrganizationName\"}")

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlAddOrganization)).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow("someId"))

	err := addOrganization(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, "someId", rec.Body.String())
}

func TestDeleteOrganizationDeleteError(t *testing.T) {
	c, rec := setupMockContext()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	forcedError := fmt.Errorf("forced org delete error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlDeleteOrganization)).
		WillReturnError(forcedError)

	err := deleteOrganization(c)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestDeleteOrganizationNotFound(t *testing.T) {
	c, rec := setupMockContext()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectExec(convertSqlToDbMockExpect(sqlDeleteOrganization)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := deleteOrganization(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, c.Response().Status)
	assert.Equal(t, "\"no organization: scpName: , name: \"\n", rec.Body.String())
}

func TestDeleteOrganization(t *testing.T) {
	c, rec := setupMockContext()

	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()
	origDb := db
	defer func() {
		db = origDb
	}()
	db = dbMock

	mock.ExpectExec(convertSqlToDbMockExpect(sqlDeleteOrganization)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := deleteOrganization(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}
