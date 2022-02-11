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

var now = time.Now()

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

	assert.EqualError(t, migrateDB(dbMock, nil), "all expectations were already fulfilled, call to Query 'SELECT CURRENT_DATABASE()' with args [] was not expected in line 0: SELECT CURRENT_DATABASE()")
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

	assert.EqualError(t, migrateDB(dbMock, nil), "try lock failed in line 0: SELECT pg_advisory_lock($1) (details: all expectations were already fulfilled, call to ExecQuery 'SELECT pg_advisory_lock($1)' with args [{Name: Ordinal:1 Value:1014225327}] was not expected)")
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

	assert.NoError(t, migrateDB(dbMock, nil))
}

const timeLayout = "2006-01-02T15:04:05.000Z"

var testStartOn time.Time
var testEndOn time.Time

func init() {
	var err error
	testStartOn, err = time.Parse(timeLayout, "2021-11-01T12:00:00.000Z")
	if err != nil {
		panic(err)
	}
	testEndOn, err = time.Parse(timeLayout, "2021-11-02T12:00:00.000Z")
	if err != nil {
		panic(err)
	}
}
func setupMockContextCampaign(campaignName string) (c echo.Context, rec *httptest.ResponseRecorder) {
	return setupMockContextCampaignWithBody(campaignName, fmt.Sprintf("{ \"startOn\": \"%s\", \"endOn\": \"%s\"}",
		testStartOn.Format(timeLayout), testEndOn.Format(timeLayout)))
}
func setupMockContextCampaignWithBody(campaignName, bodyCampaign string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest("", "/", strings.NewReader(bodyCampaign))
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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced campaign error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaigns)).
		WillReturnError(forcedError)

	assert.EqualError(t, getCampaigns(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetCampaignsScanError(t *testing.T) {
	c, rec := setupMockContext()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaigns)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "startOn", "endOn", "upstream_id", "note"}).
			// force scan error due to time.Time type mismatch at CreatedOn column
			AddRow("campaignId", campaign, "badness", 1, time.Time{}, time.Time{}, "", ""))

	assert.EqualError(t, getCampaigns(c), `sql: Scan error on column index 2, name "createdOn": unsupported Scan, storing driver.Value type string into type *time.Time`)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetCampaigns(t *testing.T) {
	c, rec := setupMockContext()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaigns)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "startOn", "endOn", "upstream_id", "note"}).
			AddRow(campaignId, campaign, time.Time{}, 1, now, now, campaignUpstreamId, nil))

	assert.NoError(t, getCampaigns(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	expectedCampaigns := []campaignStruct{
		{ID: campaignId, Name: campaign, CreatedOn: time.Time{}, CreatedOrder: 1, StartOn: now, EndOn: now, UpstreamId: campaignUpstreamId},
	}
	jsonExpectedCampaign, err := json.Marshal(expectedCampaigns)
	assert.NoError(t, err)
	assert.Equal(t, string(jsonExpectedCampaign)+"\n", rec.Body.String())
}

func TestGetActiveCampaignsScanError(t *testing.T) {
	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCurrentCampaigns)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "startOn", "endOn", "upstreamId", "note"}).
			// force scan error due to time.Time type mismatch at CreatedOn column
			AddRow(campaignId, campaign, "badness", 0, now, now, "myUpstreamId", sql.NullString{}))

	activeCampaigns, err := getActiveCampaigns(now)
	assert.EqualError(t, err, `sql: Scan error on column index 2, name "createdOn": unsupported Scan, storing driver.Value type string into type *time.Time`)
	var expectedCampaigns []campaignStruct
	assert.Equal(t, expectedCampaigns, activeCampaigns)
}

func TestGetActiveCampaigns(t *testing.T) {
	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mockCurrentCampaigns(mock)

	activeCampaigns, err := getActiveCampaigns(now)
	assert.NoError(t, err)
	expectedCampaigns := []campaignStruct{
		{ID: campaignId, Name: campaign, UpstreamId: campaignUpstreamId, StartOn: now, EndOn: now},
	}
	assert.Equal(t, expectedCampaigns, activeCampaigns)
}

func mockCurrentCampaigns(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCurrentCampaigns)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "startOn", "endOn", "upstreamId", "note"}).
			AddRow(campaignId, campaign, time.Time{}, 0, now, now, campaignUpstreamId, sql.NullString{}))
}

func TestGetActiveCampaignsEchoError(t *testing.T) {
	c, rec := setupMockContextCampaign(campaign)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced campaign error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCurrentCampaigns)).
		WillReturnError(forcedError)

	assert.NoError(t, getActiveCampaignsEcho(c))
	assert.Equal(t, http.StatusBadRequest, c.Response().Status)
	assert.Equal(t, forcedError.Error(), rec.Body.String())
}

func TestGetActiveCampaignsEcho(t *testing.T) {
	c, rec := setupMockContextCampaign(campaign)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCurrentCampaigns)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "startOn", "endOn", "upstreamId", "note"}).
			AddRow(campaignId, campaign, time.Time{}, 0, now, now, "myUpstreamId", sql.NullString{}))

	assert.NoError(t, getActiveCampaignsEcho(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	expectedCampaigns := []campaignStruct{
		{ID: campaignId, Name: campaign, StartOn: now, EndOn: now, UpstreamId: "myUpstreamId"},
	}
	jsonExpectedCampaign, err := json.Marshal(expectedCampaigns)
	assert.NoError(t, err)
	assert.Equal(t, string(jsonExpectedCampaign)+"\n", rec.Body.String())
}

func TestAddCampaignErrorReadingCampaignFromRequestBody(t *testing.T) {
	c, rec := setupMockContextCampaignWithBody(campaign, "")

	assert.EqualError(t, addCampaign(c), "EOF")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddCampaignScanError(t *testing.T) {
	c, rec := setupMockContextCampaign(campaign)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced Scan error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertCampaign)).
		WithArgs(campaign, upstreamIdDeprecated, testStartOn, testEndOn).
		WillReturnError(forcedError)

	assert.EqualError(t, addCampaign(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddCampaign(t *testing.T) {
	c, rec := setupMockContextCampaign(campaign)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertCampaign)).
		WithArgs(campaign, upstreamIdDeprecated, testStartOn, testEndOn).
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).FromCSVString(campaignId))

	assert.NoError(t, addCampaign(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, campaignId, rec.Body.String())
}

func TestUpdateCampaignMissingParamCampaign(t *testing.T) {
	c, rec := setupMockContextCampaign("")

	assert.NoError(t, updateCampaign(c))
	assert.Equal(t, http.StatusBadRequest, c.Response().Status)
	assert.Equal(t, "invalid parameter campaignName: ", rec.Body.String())
}

func TestUpdateCampaignErrorReadingCampaignFromRequestBody(t *testing.T) {
	c, rec := setupMockContextCampaignWithBody(campaign, "")

	assert.EqualError(t, updateCampaign(c), "EOF")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateCampaignScanError(t *testing.T) {
	c, rec := setupMockContextCampaign(campaign)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced scan error update campaign")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateCampaign)).
		WithArgs(testStartOn, testEndOn, campaign).
		WillReturnError(forcedError)

	assert.EqualError(t, updateCampaign(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func mockUpdateCampaign(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateCampaign)).
		WithArgs(testStartOn, testEndOn, campaign).
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).AddRow(campaignId))
}

func TestUpdateCampaign(t *testing.T) {
	c, rec := setupMockContextCampaign(campaign)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mockUpdateCampaign(mock)

	mockSelectCampaigns(mock)

	assert.NoError(t, updateCampaign(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.Equal(t, campaignId, rec.Body.String())
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

func TestGetCampaignQueryError(t *testing.T) {
	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced campaign query error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaigns)).
		WithArgs(campaign).
		WillReturnError(forcedError)

	actualCampaign, err := getCampaign(campaign)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, campaignStruct{}, actualCampaign)
}

func TestGetCampaignScanError(t *testing.T) {
	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaigns)).
		WithArgs(campaign).
		WillReturnRows(sqlmock.NewRows([]string{"ID", "name", "created_on", "create_order", "start_on", "end_on", "upstream_id", "note"}).
			// force scan error due to invalid type
			AddRow(campaignId, campaign, "", 0, now, now, campaignUpstreamId, sql.NullString{}))

	actualCampaign, err := getCampaign(campaign)
	assert.EqualError(t, err, "sql: Scan error on column index 2, name \"created_on\": unsupported Scan, storing driver.Value type string into type *time.Time")
	// note this struct is partially populated
	assert.Equal(t, campaignStruct{ID: campaignId, Name: campaign}, actualCampaign)
}

func TestGetCampaign(t *testing.T) {
	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mockSelectCampaigns(mock)

	actualCampaign, err := getCampaign(campaign)
	assert.NoError(t, err)
	assert.Equal(t, campaignStruct{
		ID:           campaignId,
		Name:         campaign,
		CreatedOn:    now,
		CreatedOrder: 0,
		StartOn:      now,
		EndOn:        now,
		UpstreamId:   campaignUpstreamId,
		Note:         sql.NullString{},
	}, actualCampaign)
}

func mockSelectCampaigns(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaigns)).
		WithArgs(campaign).
		WillReturnRows(sqlmock.NewRows([]string{"ID", "name", "created_on", "create_order", "start_on", "end_on", "upstream_id", "note"}).
			AddRow(campaignId, campaign, now, 0, now, now, campaignUpstreamId, sql.NullString{}))
}

func setupMockDb(t *testing.T) (mock sqlmock.Sqlmock, resetMockDb func()) {
	var dbMock *sql.DB
	dbMock, mock = newMockDb(t)
	origDb := db
	db = dbMock

	resetMockDb = func() {
		_ = dbMock.Close()
		db = origDb
	}
	return
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

func TestAddParticipantCampaignMissing(t *testing.T) {
	participantJson := fmt.Sprintf(`{"campaignName":"%s", "loginName": "%s"}`, campaign, loginName)
	c, rec := setupMockContextParticipant(participantJson)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced SQL insert error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertParticipant)).
		WillReturnError(forcedError)

	assert.EqualError(t, addParticipant(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddParticipant(t *testing.T) {
	participantJson := fmt.Sprintf(`{"campaignName":"%s", "scpName": "%s","loginName": "%s"}`, campaign, scpName, loginName)
	c, rec := setupMockContextParticipant(participantJson)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertParticipant)).
		WithArgs(scpName, campaign, loginName, "", "", 0, upstreamIdDeprecated).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "Score", "JoinedAt"}).AddRow(participantID, 0, time.Time{}))

	assert.NoError(t, addParticipant(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `{"guid":"`+participantID+`","endpoints":{"participantDetail"`), rec.Body.String())
	assert.True(t, strings.Contains(rec.Body.String(), `"loginName":"`+loginName+`"`), rec.Body.String())
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

	assert.NoError(t, migrateDB(db, nil))
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
	participantJson := fmt.Sprintf(`{"campaignName":"%s", "scpName": "%s","loginName": "%s"}`, campaign, scpName, loginName)
	c, rec := setupMockContextParticipant(participantJson)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertParticipant)).
		WithArgs(scpName, campaign, loginName, "", "", 0, upstreamIdDeprecated).
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
const tokenUpstream = "testWfToken"
const campaignUpstreamCollection = "testWfCollectionCampaign"
const campaignUpstreamId = "myCampaignUpstreamId"
const scpName = "myScpName"
const participantID = "participantUUId"
const participantUpstreamCollection = "testWfCollectionParticipant"
const participantUpstreamId = "myParticipantUpstreamId"
const loginName = "loginName"
const teamName = "myTeamName"

func TestUpdateParticipantMissingParticipantID(t *testing.T) {
	participantJson := fmt.Sprintf(`{"loginName": "%s","campaignName": "%s", "scpName": "%s"}`, loginName, campaign, scpName)
	c, rec := setupMockContextUpdateParticipant(participantJson)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced SQL insert error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(campaign, scpName, loginName, "", "", 0, "", "").
		WillReturnError(forcedError)

	assert.EqualError(t, updateParticipant(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateParticipantUpdateError(t *testing.T) {
	participantJson := fmt.Sprintf(`{"guid": "%s","campaignName": "%s", "scpName": "%s", "loginName": "%s"}`, participantID, campaign, scpName, loginName)
	c, rec := setupMockContextUpdateParticipant(participantJson)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced SQL insert error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(campaign, scpName, loginName, "", "", 0, "", participantID).
		WillReturnResult(sqlmock.NewErrorResult(forcedError))

	assert.EqualError(t, updateParticipant(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateParticipantScoreError(t *testing.T) {
	participantJson := fmt.Sprintf(`{"guid": "%s","campaignName": "%s", "scpName": "%s", "loginName": "%s"}`, participantID, campaign, scpName, loginName)
	c, rec := setupMockContextUpdateParticipant(participantJson)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(campaign, scpName, loginName, "", "", 0, "", participantID).
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

func TestUpdateParticipantNoRowsUpdated(t *testing.T) {
	participantJson := fmt.Sprintf(`{"guid": "%s", "campaignName": "%s", "scpName": "%s", "loginName": "%s", "teamName": "%s"}`, participantID, campaign, scpName, loginName, teamName)
	c, rec := setupMockContextUpdateParticipant(participantJson)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(campaign, scpName, loginName, "", "", 0, teamName, participantID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(0, participantID).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(upstreamIdDeprecated, 0))

	assert.NoError(t, updateParticipant(c))
	assert.Equal(t, http.StatusBadRequest, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateParticipant(t *testing.T) {
	participantJson := fmt.Sprintf(`{"guid": "%s", "campaignName": "%s", "scpName": "%s", "loginName": "%s"}`, participantID, campaign, scpName, loginName)
	c, rec := setupMockContextUpdateParticipant(participantJson)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(campaign, scpName, loginName, "", "", 0, "", participantID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(0, participantID).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(upstreamIdDeprecated, 0))

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced SQL insert error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertTeam)).
		WillReturnError(forcedError)

	assert.EqualError(t, addTeam(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddTeamEmptyOrganization(t *testing.T) {
	teamJson := `{"campaignName": "` + campaign + `","name": "` + teamName + `"}`
	c, rec := setupMockContextTeam(teamJson)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	teamID := "teamUUId"
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertTeam)).
		WithArgs(campaign, teamName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow(teamID))

	assert.NoError(t, addTeam(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, teamID, rec.Body.String())
}

func TestAddTeam(t *testing.T) {
	teamJson := `{"campaignName": "` + campaign + `","name":"` + teamName + `"}`
	c, rec := setupMockContextTeam(teamJson)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced SQL update error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipantTeam)).
		WillReturnError(forcedError)

	assert.EqualError(t, addPersonToTeam(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddPersonToTeamRowsAffectedError(t *testing.T) {
	c, rec := setupMockContextAddPersonToTeam(campaign, scpName, loginName, teamName)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipantTeam)).
		WithArgs(teamName, campaign, scpName, loginName).
		WillReturnResult(sqlmock.NewResult(0, 0))

	assert.NoError(t, addPersonToTeam(c))
	assert.Equal(t, http.StatusBadRequest, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddPersonToTeamSomeRowsAffected(t *testing.T) {
	c, rec := setupMockContextAddPersonToTeam(campaign, scpName, loginName, teamName)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantDetail)).
		WithArgs(campaign, scpName, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignUpstreamId", "SCPName", "LoginName", "Email", "DisplayName", "Score", "TeamName", "JoinedAt"}).AddRow(participantID, campaign, scpName, loginName, "", "", 0, "", time.Time{}))

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantsByCampaign)).
		WithArgs(campaign).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignUpstreamId", "SCPName", "LoginName", "Email", "DisplayName", "Score", "TeamName", "JoinedAt"}).
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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced Select error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectBug)).WillReturnError(forcedError)

	assert.EqualError(t, getBugs(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetBugsScanError(t *testing.T) {
	c, rec := setupMockContextGetBugs()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced Begin Txn error")
	mock.ExpectBegin().WillReturnError(forcedError)

	assert.EqualError(t, putBugs(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestPutBugsScanError(t *testing.T) {
	c, rec := setupMockContextPutBugs(`[{"campaign":"myCampaign","category":"bugCat2", "pointvalue":5}]`)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectBegin()

	assert.EqualError(t, putBugs(c), "bug is not valid, empty campaign: bug: {Id: Campaign: Category: PointValue:0}")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}
func TestPutBugsOneBug(t *testing.T) {
	c, rec := setupMockContextPutBugs(`[{"campaign":"myCampaign","category":"bugCat2", "pointvalue":5}]`)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

func setupMockContextParticipantDelete(campaignName, scpName, loginName string) (c echo.Context, rec *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetParamNames(ParamCampaignName, ParamScpName, ParamLoginName)
	c.SetParamValues(campaignName, scpName, loginName)
	return
}

func TestDeleteParticipant(t *testing.T) {
	c, rec := setupMockContextParticipantDelete(campaign, scpName, loginName)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlDeleteParticipant)).
		WithArgs(campaign, scpName, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"upstreamid"}).AddRow(upstreamIdDeprecated))

	assert.NoError(t, deleteParticipant(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.Equal(t, fmt.Sprintf("\"deleted participant: campaign: %s, scpName: %s, loginName: %s, participantUpstreamId: %s\"\n", campaign, scpName, loginName, upstreamIdDeprecated), rec.Body.String())
}

func TestDeleteParticipantWithDBDeleteError(t *testing.T) {
	c, rec := setupMockContextParticipantDelete(campaign, scpName, loginName)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced delete error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlDeleteParticipant)).
		WithArgs(campaign, scpName, loginName).
		WillReturnError(forcedError)

	assert.EqualError(t, deleteParticipant(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestValidScoreUnknownErrorValidatingOrganization(t *testing.T) {
	c, _ := setupMockContext()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced org exists query error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs(testEventSourceValid, testOrgValid).
		WillReturnError(forcedError)

	msg := scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid}
	activeParticipantsToScore, err := validScore(c, msg, now)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, 0, len(activeParticipantsToScore))
}

func TestValidScoreUnknownRepoOwner(t *testing.T) {
	c, _ := setupMockContext()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs(testEventSourceValid, testOrgValid).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	msg := scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid}
	activeParticipantsToScore, err := validScore(c, msg, now)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(activeParticipantsToScore))
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
	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	setupMockDBOrgValid(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(now, testEventSourceValid, "unregisteredUser").
		WillReturnRows(sqlmock.NewRows([]string{"Id"}))

	c, _ := setupMockContext()

	msg := scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: "unregisteredUser"}
	activeParticipantsToScore, err := validScore(c, msg, now)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(activeParticipantsToScore))
}

func TestValidScoreParticipantScanError(t *testing.T) {
	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	setupMockDBOrgValid(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(now, testEventSourceValid, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignName", "SCPName", "loginName", "teamName"}).
			// force scan error via invalid datatype
			AddRow("someId", sql.NullString{}, "someSCP", "someLoginName", ""))

	c, _ := setupMockContext()

	msg := scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName}
	activeParticipantsToScore, err := validScore(c, msg, now)
	assert.EqualError(t, err, "sql: Scan error on column index 1, name \"CampaignName\": converting NULL to string is unsupported")
	assert.Equal(t, 0, len(activeParticipantsToScore))
}

func TestValidScoreParticipantErrorReadingParticipant(t *testing.T) {
	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	setupMockDBOrgValid(mock)

	forcedError := fmt.Errorf("forced current campaign read error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(now, testEventSourceValid, loginName).
		WillReturnError(forcedError)

	c, _ := setupMockContext()

	msg := scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName}
	activeParticipantsToScore, err := validScore(c, msg, now)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, 0, len(activeParticipantsToScore))
}

func TestValidScoreParticipant(t *testing.T) {
	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	setupMockDBOrgValid(mock)

	//mockCurrentCampaigns(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(now, testEventSourceValid, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignName", "SCPName", "loginName", "teamName"}).
			AddRow("someId", "someCampaign", "someSCP", "someLoginName", ""))

	c, _ := setupMockContext()

	msg := scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName}
	activeParticipantsToScore, err := validScore(c, msg, now)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(activeParticipantsToScore))
	assert.Equal(t, "someCampaign", activeParticipantsToScore[0].CampaignName)
	assert.Equal(t, "someSCP", activeParticipantsToScore[0].ScpName)
}

func setupMockDBOrgValid(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs(testEventSourceValid, testOrgValid).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
}

func TestScorePointsNothing(t *testing.T) {
	msg := scoringMessage{}
	points := scorePoints(nil, msg, campaign)
	assert.Equal(t, 0, points)
}

func TestScorePointsScanError(t *testing.T) {
	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	msg := scoringMessage{BugCounts: map[string]int{"myBugType": 1}}
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectPointValue)).
		WithArgs("unexpectedBugType").
		WillReturnRows(sqlmock.NewRows([]string{"Value"}).AddRow(1))

	c, _ := setupMockContext()

	points := scorePoints(c, msg, campaign)
	assert.Equal(t, 1, points)
}

func TestScorePointsFixedTwoThreePointers(t *testing.T) {
	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	bugType := "threePointBugType"
	msg := scoringMessage{BugCounts: map[string]int{bugType: 2}}
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectPointValue)).
		WithArgs(campaign, bugType).
		WillReturnRows(sqlmock.NewRows([]string{"Value"}).AddRow(3))

	points := scorePoints(nil, msg, campaign)
	assert.Equal(t, 6, points)
}

func TestScorePointsBonusForNonClassified(t *testing.T) {
	msg := scoringMessage{TotalFixed: 1}
	points := scorePoints(nil, msg, campaign)
	assert.Equal(t, 1, points)
}

func TestValidOrganizationFalse(t *testing.T) {
	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs(testEventSourceValid, testOrgValid).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	c, _ := setupMockContext()
	msg := scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid}
	isValidOrg, err := validOrganization(c, msg)
	assert.Nil(t, err)
	assert.False(t, isValidOrg)
}

func TestValidOrganizationError(t *testing.T) {
	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced org exists query error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs("GitHub", testOrgValid).
		WillReturnError(forcedError)

	c, _ := setupMockContext()
	msg := scoringMessage{EventSource: "GitHub", RepoOwner: testOrgValid}
	isValidOrg, err := validOrganization(c, msg)
	assert.EqualError(t, err, forcedError.Error())
	assert.False(t, isValidOrg)
}

func TestValidOrganization(t *testing.T) {
	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs(testEventSourceValid, testOrgValid).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	c, _ := setupMockContext()
	msg := scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid}
	isValidOrg, err := validOrganization(c, msg)
	assert.Nil(t, err)
	assert.True(t, isValidOrg)
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

func TestNewScoreOneAlertInvalidScore_Errro(t *testing.T) {
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	setupMockDBOrgValid(mock)

	forcedError := fmt.Errorf("forced validScore error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(sqlmock.AnyArg(), testEventSourceValid, strings.ToLower(loginName)).
		WillReturnError(forcedError)

	err = newScore(c)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertInvalidScore_NoTriggerUserFound(t *testing.T) {
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	setupMockDBOrgValid(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(sqlmock.AnyArg(), testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{}))

	err = newScore(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertHandleBeginTransactionError(t *testing.T) {
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	setupMockDBOrgValid(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(sqlmock.AnyArg(), testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignName", "SCPName", "loginName", "teamName"}).
			AddRow("someId", "someCampaign", testEventSourceValid, "someLoginName", ""))

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	setupMockDBOrgValid(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(sqlmock.AnyArg(), testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignName", "SCPName", "loginName", "teamName"}).
			AddRow("someId", "someCampaign", testEventSourceValid, "someLoginName", ""))

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
	repoName := "myRepoName"
	prId := -5
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName, RepoName: repoName, PullRequest: prId})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	setupMockDBOrgValid(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(sqlmock.AnyArg(), testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignName", "SCPName", "loginName", "teamName"}).
			AddRow("someId", campaign, testEventSourceValid, "someLoginName", ""))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(campaign, testEventSourceValid, testOrgValid, repoName, prId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	err = newScore(c)
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "all expectations were already fulfilled, call to ExecQuery 'INSERT INTO scoring_event"))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertUpdateScoreErrorNotIgnored(t *testing.T) {
	repoName := "myRepoName"
	prId := -5
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName, RepoName: repoName, PullRequest: prId})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	setupMockDBOrgValid(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(sqlmock.AnyArg(), testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignName", "SCPName", "loginName", "teamName"}).
			AddRow("someId", campaign, testEventSourceValid, "someLoginName", ""))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(campaign, testEventSourceValid, testOrgValid, repoName, prId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	mock.ExpectExec(convertSqlToDbMockExpect(sqlInsertScoringEvent)).
		WithArgs(campaign, testEventSourceValid, testOrgValid, repoName, prId, strings.ToLower(loginName), 0).
		WillReturnResult(sqlmock.NewResult(0, -1))

	err = newScore(c)
	assert.NotNil(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "all expectations were already fulfilled, call to Query 'UPDATE participant"))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertCommitErrorNotIgnored(t *testing.T) {
	repoName := "myRepoName"
	prId := -5
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName, RepoName: repoName, PullRequest: prId})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	setupMockDBOrgValid(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(sqlmock.AnyArg(), testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignName", "SCPName", "loginName", "teamName"}).
			AddRow(participantID, campaign, testEventSourceValid, "someLoginName", ""))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(campaign, testEventSourceValid, testOrgValid, repoName, prId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	mock.ExpectExec(convertSqlToDbMockExpect(sqlInsertScoringEvent)).
		WithArgs(campaign, testEventSourceValid, testOrgValid, repoName, prId, strings.ToLower(loginName), 0).
		WillReturnResult(sqlmock.NewResult(0, -1))

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(8, participantID).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(strings.ToLower(loginName), 0))

	err = newScore(c)
	assert.EqualError(t, err, "all expectations were already fulfilled, call to Commit transaction was not expected")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertUserCapitalizationMismatch(t *testing.T) {
	loginName := "MYGithubName"
	loginNameLowerCase := strings.ToLower(loginName)
	repoName := "myRepoName"
	prId := -5
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName, RepoName: repoName, PullRequest: prId})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	setupMockDBOrgValid(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(sqlmock.AnyArg(), testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignName", "SCPName", "loginName", "teamName"}).
			AddRow(participantID, campaign, testEventSourceValid, loginName, ""))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(campaign, testEventSourceValid, testOrgValid, repoName, prId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	mock.ExpectExec(convertSqlToDbMockExpect(sqlInsertScoringEvent)).
		WithArgs(campaign, testEventSourceValid, testOrgValid, repoName, prId, loginNameLowerCase, 0).
		WillReturnResult(sqlmock.NewResult(0, -1))

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(8, participantID).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(loginNameLowerCase, 0))

	mock.ExpectCommit()

	err = newScore(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlert(t *testing.T) {
	repoName := "myRepoName"
	prId := -5
	scoringMsgBytes, err := json.Marshal(scoringMessage{EventSource: testEventSourceValid, RepoOwner: testOrgValid, TriggerUser: loginName, RepoName: repoName, PullRequest: prId})
	assert.NoError(t, err)
	scoringMsgJson := string(scoringMsgBytes)
	c, rec := setupMockContextNewScore(t, scoringAlert{
		RecentHits: []string{scoringMsgJson},
	})

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	setupMockDBOrgValid(mock)

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(sqlmock.AnyArg(), testEventSourceValid, strings.ToLower(loginName)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignName", "SCPName", "loginName", "teamName"}).
			AddRow(participantID, campaign, testEventSourceValid, loginName, ""))

	mock.ExpectBegin()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(campaign, testEventSourceValid, testOrgValid, repoName, prId).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow("-8"))

	mock.ExpectExec(convertSqlToDbMockExpect(sqlInsertScoringEvent)).
		WithArgs(campaign, testEventSourceValid, testOrgValid, repoName, prId, strings.ToLower(loginName), 0).
		WillReturnResult(sqlmock.NewResult(0, -1))

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(8, participantID).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(loginName, 0))

	mock.ExpectCommit()

	err = newScore(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetSourceControlProvidersQueryError(t *testing.T) {
	c, rec := setupMockContext()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectSourceControlProvider)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "name", "url"}).AddRow("someId", "someSCP", "someUrl"))

	err := getSourceControlProviders(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.Equal(t, "[{\"guid\":\"someId\",\"scpName\":\"someSCP\",\"url\":\"someUrl\"}]\n", rec.Body.String())
}

func TestGetOrganizationsError(t *testing.T) {
	c, rec := setupMockContext()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganization)).
		WillReturnRows(sqlmock.NewRows([]string{}).AddRow())

	err := getOrganizations(c)
	assert.EqualError(t, err, "sql: expected 0 destination arguments in Scan, not 3")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetOrganizations(t *testing.T) {
	c, rec := setupMockContext()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlAddOrganization)).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow("someId"))

	err := addOrganization(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, "someId", rec.Body.String())
}

func TestDeleteOrganizationDeleteError(t *testing.T) {
	c, rec := setupMockContext()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

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

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectExec(convertSqlToDbMockExpect(sqlDeleteOrganization)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := deleteOrganization(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, c.Response().Status)
	assert.Equal(t, "\"no organization: scpName: , name: \"\n", rec.Body.String())
}

func TestDeleteOrganization(t *testing.T) {
	c, rec := setupMockContext()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectExec(convertSqlToDbMockExpect(sqlDeleteOrganization)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := deleteOrganization(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}
