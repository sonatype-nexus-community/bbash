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

//go:build go1.16
// +build go1.16

package poll

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-api-client-go/api/v2/datadog"
	"github.com/joho/godotenv"
	"github.com/sonatype-nexus-community/bbash/internal/db"
	"github.com/sonatype-nexus-community/bbash/internal/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

type MockDogApiClient struct {
	mockUrl *url.URL
}

var _ IDogApiClient = (*MockDogApiClient)(nil)

func (c *MockDogApiClient) getDDApiClient() (ctx context.Context, apiClient *datadog.APIClient) {
	configuration := datadog.NewConfiguration()
	configuration.Servers = datadog.ServerConfigurations{
		datadog.ServerConfiguration{
			URL:         "{protocol}://{name}",
			Description: "No description provided",
			Variables: map[string]datadog.ServerVariable{
				"name": {
					Description:  "Full site DNS name.",
					DefaultValue: c.mockUrl.Host,
				},
				"protocol": {
					Description:  "The protocol for accessing the API.",
					DefaultValue: "http",
				},
			},
		},
	}
	apiClient = datadog.NewAPIClient(configuration)

	ctx = context.Background()
	return
}

func setupMockDDogApiClient(mockUrl *url.URL) (closeApiClient func()) {
	origDogApiClient := dogApiClient
	closeApiClient = func() {
		dogApiClient = origDogApiClient
	}

	dogApiClient = &MockDogApiClient{
		mockUrl: mockUrl,
	}
	return
}

func TestGetDDApiClientReal(t *testing.T) {
	contextReal, clientReal := dogApiClient.getDDApiClient()
	assert.NotNil(t, contextReal)
	assert.Equal(t, 3, len(clientReal.GetConfig().Servers))
	assert.Equal(t, "https://{subdomain}.{site}", clientReal.GetConfig().Servers[0].URL)
}

func TestGetDDApiClientRealHasSomeScoresInPastWeek(t *testing.T) {
	// skip if not running nightly CI job
	ddApiKey := os.Getenv("DD_CLIENT_API_KEY")
	if "" == ddApiKey {
		fmt.Println("skipping test: TestGetDDApiClientRealHasSomeScoresInPastWeek")
		return
	}

	logger = zaptest.NewLogger(t)

	pageCursor := ""
	isDone := false
	var logPage []ddLog
	var err error

	now := time.Now()
	hoursDuration := time.Hour * -168 // one week in the past
	before := now.Add(hoursDuration)

	isDone, pageCursor, logPage, _, err = fetchLogPage(before, now, &pageCursor)
	foundInfo := fmt.Sprintf("found logCount: %d in the past: %v", len(logPage), hoursDuration)
	fmt.Println(foundInfo)

	assert.NoError(t, err)
	assert.True(t, len(logPage) > 0)
	assert.True(t, isDone)
	assert.Equal(t, "", pageCursor)
}

func TestFetchLogPagesErrorMissingKey(t *testing.T) {
	logger = zaptest.NewLogger(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	urlTs, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	closeApiClient := setupMockDDogApiClient(urlTs)
	defer closeApiClient()

	now := time.Now()
	pageCursor := ""
	var logPage []ddLog

	isDone, cursor, logPage, _, err := fetchLogPage(now, now, &pageCursor)
	assert.False(t, isDone)
	assert.Equal(t, "", cursor)
	assert.Equal(t, ([]ddLog)(nil), logPage)
	//assert.EqualError(t, err, "403 Forbidden")
	assert.EqualError(t, err, "500 Internal Server Error")
}

func TestFetchLogPagesMetaWarnings(t *testing.T) {
	logger = zaptest.NewLogger(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)

		title := "forcedWarningTitle"
		warnings := datadog.LogsWarning{
			Title: &title,
		}
		resp := datadog.LogsListResponse{
			Meta: &datadog.LogsResponseMetadata{
				Warnings: &[]datadog.LogsWarning{warnings},
			},
		}
		jsonWarnings, err := json.Marshal(resp)
		assert.NoError(t, err)
		_, _ = w.Write(jsonWarnings)
	}))
	defer ts.Close()
	urlTs, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	closeApiClient := setupMockDDogApiClient(urlTs)
	defer closeApiClient()

	now := time.Now()
	pageCursor := ""
	var logPage []ddLog

	isDone, cursor, logPage, _, err := fetchLogPage(now, now, &pageCursor)
	assert.False(t, isDone)
	assert.Equal(t, "", cursor)
	assert.Equal(t, ([]ddLog)(nil), logPage)
	assert.EqualError(t, err, "datadog warnings: 1, see log")
}

func TestFetchLogPagesMetaStatusTimeout(t *testing.T) {
	logger = zaptest.NewLogger(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)

		status := datadog.LOGSAGGREGATERESPONSESTATUS_TIMEOUT
		resp := datadog.LogsListResponse{
			Meta: &datadog.LogsResponseMetadata{
				Status: &status,
			},
		}
		jsonWarnings, err := json.Marshal(resp)
		assert.NoError(t, err)
		_, _ = w.Write(jsonWarnings)
	}))
	defer ts.Close()
	urlTs, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	closeApiClient := setupMockDDogApiClient(urlTs)
	defer closeApiClient()

	now := time.Now()
	pageCursor := ""
	var logPage []ddLog

	isDone, cursor, logPage, _, err := fetchLogPage(now, now, &pageCursor)
	assert.False(t, isDone)
	assert.Equal(t, "", cursor)
	assert.Equal(t, ([]ddLog)(nil), logPage)
	assert.EqualError(t, err, "timeout getting scoring page. timeout")
}

func TestFetchLogPagesMetaStatusDone(t *testing.T) {
	logger = zaptest.NewLogger(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)

		status := datadog.LOGSAGGREGATERESPONSESTATUS_DONE
		resp := datadog.LogsListResponse{
			Meta: &datadog.LogsResponseMetadata{
				Status: &status,
			},
		}
		jsonWarnings, err := json.Marshal(resp)
		assert.NoError(t, err)
		_, _ = w.Write(jsonWarnings)
	}))
	defer ts.Close()
	urlTs, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	closeApiClient := setupMockDDogApiClient(urlTs)
	defer closeApiClient()

	now := time.Now()
	pageCursor := ""
	var logPage []ddLog

	isDone, cursor, logPage, fetchDuration, err := fetchLogPage(now, now, &pageCursor)
	assert.True(t, isDone)
	assert.Equal(t, "", cursor)
	assert.Equal(t, ([]ddLog)(nil), logPage)
	assert.Less(t, fetchDuration.Milliseconds(), time.Duration(3))
	assert.NoError(t, err)
}

func TestFetchLogPagesMetaPageHasAfter(t *testing.T) {
	logger = zaptest.NewLogger(t)

	after := "myAfter"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)

		page := datadog.LogsResponseMetadataPage{
			After: &after,
		}
		resp := datadog.LogsListResponse{
			Meta: &datadog.LogsResponseMetadata{
				Page: &page,
			},
		}
		jsonWarnings, err := json.Marshal(resp)
		assert.NoError(t, err)
		_, _ = w.Write(jsonWarnings)
	}))
	defer ts.Close()
	urlTs, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	closeApiClient := setupMockDDogApiClient(urlTs)
	defer closeApiClient()

	now := time.Now()
	pageCursor := ""
	var logPage []ddLog

	isDone, cursor, logPage, _, err := fetchLogPage(now, now, &pageCursor)
	assert.False(t, isDone)
	assert.Equal(t, after, cursor)
	assert.Equal(t, ([]ddLog)(nil), logPage)
	assert.NoError(t, err)
}

func TestFetchLogPagesMetaPageNoAfter(t *testing.T) {
	logger = zaptest.NewLogger(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)

		resp := datadog.LogsListResponse{}
		jsonWarnings, err := json.Marshal(resp)
		assert.NoError(t, err)
		_, _ = w.Write(jsonWarnings)
	}))
	defer ts.Close()
	urlTs, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	closeApiClient := setupMockDDogApiClient(urlTs)
	defer closeApiClient()

	now := time.Now()
	pageCursor := ""
	var logPage []ddLog

	isDone, cursor, logPage, _, err := fetchLogPage(now, now, &pageCursor)
	assert.True(t, isDone)
	assert.Equal(t, "", cursor)
	assert.Equal(t, ([]ddLog)(nil), logPage)
	assert.NoError(t, err)
}

func TestFetchLogPagesWithCursor(t *testing.T) {
	logger = zaptest.NewLogger(t)

	pageCursor := "myPageCursor"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)

		body, err := ioutil.ReadAll(r.Body)
		assert.NoError(t, err)
		logsRequest := datadog.LogsListRequest{}
		err = json.Unmarshal(body, &logsRequest)
		assert.NoError(t, err)
		assert.Equal(t, &pageCursor, logsRequest.Page.Cursor)

		w.WriteHeader(http.StatusOK)

		resp := datadog.LogsListResponse{}
		jsonWarnings, err := json.Marshal(resp)
		assert.NoError(t, err)
		_, _ = w.Write(jsonWarnings)
	}))
	defer ts.Close()
	urlTs, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	closeApiClient := setupMockDDogApiClient(urlTs)
	defer closeApiClient()

	now := time.Now()
	var logPage []ddLog

	isDone, cursor, logPage, _, err := fetchLogPage(now, now, &pageCursor)
	assert.True(t, isDone)
	assert.Equal(t, "", cursor)
	assert.Equal(t, ([]ddLog)(nil), logPage)
	assert.NoError(t, err)
}

func TestProcessResponseDataEmpty(t *testing.T) {
	logs, err := processResponseData([]datadog.Log{})
	assert.Equal(t, 0, len(logs))
	assert.NoError(t, err)
}

func TestProcessResponseDataMissingEnvMap(t *testing.T) {
	logId := "myLogId"
	attribs := map[string]interface{}{}
	logAttribs := datadog.LogAttributes{
		Attributes: attribs,
	}
	responseData := []datadog.Log{
		{
			Id:             &logId,
			Attributes:     &logAttribs,
			Type:           nil,
			UnparsedObject: nil,
		},
	}
	logs, err := processResponseData(responseData)
	assert.Equal(t, 0, len(logs))
	assert.EqualError(t, err, "unexpected attribute map type in map[]")
}

func TestProcessResponseDataUnexpectedMapKey(t *testing.T) {
	logId := "myLogId"
	envMap := map[string]interface{}{
		"yadda": "myEnv",
	}
	attribs := map[string]interface{}{
		qryEnv: envMap,
	}
	logAttribs := datadog.LogAttributes{
		Attributes: attribs,
	}
	responseData := []datadog.Log{
		{
			Id:             &logId,
			Attributes:     &logAttribs,
			Type:           nil,
			UnparsedObject: nil,
		},
	}
	logs, err := processResponseData(responseData)
	assert.Equal(t, 0, len(logs))
	assert.EqualError(t, err, "unexpected extra field key: yadda")
}

func TestProcessResponseDataMapKeyBaseTimeFormatError(t *testing.T) {
	logId := "myLogId"
	now := time.Now()
	envMap := map[string]interface{}{
		qryEnvBaseTime: now.Format(time.RFC3339) + "yadda",
	}
	attribs := map[string]interface{}{
		qryEnv: envMap,
	}
	logAttribs := datadog.LogAttributes{
		Attributes: attribs,
	}
	responseData := []datadog.Log{
		{
			Id:             &logId,
			Attributes:     &logAttribs,
			Type:           nil,
			UnparsedObject: nil,
		},
	}
	logs, err := processResponseData(responseData)
	assert.Equal(t, 0, len(logs))
	assert.True(t, strings.HasPrefix(err.Error(), "parsing time "))
}

func TestProcessResponseDataMapKeyBaseTime(t *testing.T) {
	logId := "myLogId"
	now := time.Now()
	envMap := map[string]interface{}{
		qryEnvBaseTime: now.Format(time.RFC3339),
	}
	attribs := map[string]interface{}{
		qryEnv: envMap,
	}
	logAttribs := datadog.LogAttributes{
		Attributes: attribs,
	}
	responseData := []datadog.Log{
		{
			Id:             &logId,
			Attributes:     &logAttribs,
			Type:           nil,
			UnparsedObject: nil,
		},
	}
	logs, err := processResponseData(responseData)
	assert.Equal(t, 1, len(logs))
	assert.NoError(t, err)

	nowFormatted, err := time.Parse(time.RFC3339, now.Format(time.RFC3339))
	assert.NoError(t, err)
	assert.Equal(t, nowFormatted, logs[0].Fields.envBaseTime)
}

func TestProcessResponseDataScoringMessageUnexpectedMarshalError(t *testing.T) {
	logId := "myLogId"
	mapExtraFields := map[string]interface{}{
		"fixed-bug-types": "notAMapLikeWeExpect",
	}
	envMap := map[string]interface{}{
		qryEnvExtraJsonFields: mapExtraFields,
	}
	attribs := map[string]interface{}{
		qryEnv: envMap,
	}
	logAttribs := datadog.LogAttributes{
		Attributes: attribs,
	}
	responseData := []datadog.Log{
		{
			Id:             &logId,
			Attributes:     &logAttribs,
			Type:           nil,
			UnparsedObject: nil,
		},
	}

	logger = zaptest.NewLogger(t)

	logs, err := processResponseData(responseData)
	assert.Equal(t, 0, len(logs))
	assert.EqualError(t, err, "json: cannot unmarshal string into Go struct field ScoringMessage.fixed-bug-types of type map[string]interface {}")
}

func TestProcessResponseDataScoringMessageFixedBugsWithOptMap(t *testing.T) {
	// this map is odd, but allowed. assumes leaf keys are unique for assigning score values
	mapSprintf := map[string]interface{}{"sprintf-host-port": float64(2)}
	mapSemGrep := map[string]interface{}{"semgrep": mapSprintf}
	mapBugTypes := map[string]interface{}{
		"G104":       1,
		"ShellCheck": 1,
		"opt":        mapSemGrep,
	}
	mapExtraFields := map[string]interface{}{
		"fixed-bug-types": mapBugTypes,
	}
	envMap := map[string]interface{}{qryEnvExtraJsonFields: mapExtraFields}
	attribs := map[string]interface{}{qryEnv: envMap}
	logAttribs := datadog.LogAttributes{Attributes: attribs}
	logId := "myLogId"
	responseData := []datadog.Log{
		{
			Id:             &logId,
			Attributes:     &logAttribs,
			Type:           nil,
			UnparsedObject: nil,
		},
	}

	logger = zaptest.NewLogger(t)

	logs, err := processResponseData(responseData)
	assert.Equal(t, 1, len(logs))
	assert.NoError(t, err)
	assert.Equal(t, float64(1), logs[0].Fields.scoringMessage.BugCounts["G104"])
	assert.Equal(t, float64(1), logs[0].Fields.scoringMessage.BugCounts["ShellCheck"])
	assert.Equal(t, mapSemGrep, logs[0].Fields.scoringMessage.BugCounts["opt"])
}

func TestProcessResponseDataScoringMessage(t *testing.T) {
	logId := "myLogId"

	mapBugTypes := map[string]interface{}{
		"G104":       1,
		"ShellCheck": 2,
	}
	mapExtraFields := map[string]interface{}{
		"fixed-bug-types": mapBugTypes,
	}
	envMap := map[string]interface{}{
		qryEnvExtraJsonFields: mapExtraFields,
	}
	attribs := map[string]interface{}{
		qryEnv: envMap,
	}
	logAttribs := datadog.LogAttributes{
		Attributes: attribs,
	}
	responseData := []datadog.Log{
		{
			Id:             &logId,
			Attributes:     &logAttribs,
			Type:           nil,
			UnparsedObject: nil,
		},
	}

	logger = zaptest.NewLogger(t)

	logs, err := processResponseData(responseData)
	assert.Equal(t, 1, len(logs))
	assert.NoError(t, err)
	assert.Equal(t, float64(1), logs[0].Fields.scoringMessage.BugCounts["G104"])
	assert.Equal(t, float64(2), logs[0].Fields.scoringMessage.BugCounts["ShellCheck"])
}

func TestPollTheDogDBError(t *testing.T) {
	logger = zaptest.NewLogger(t)

	mock, dbPoll, closeDbFunc := db.SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := dbPoll.NewPoll()
	forcedError := fmt.Errorf("forced poll db error")
	db.SetupMockPollSelectForcedError(mock, forcedError, poll.Id)

	now := time.Now()
	logs, err := pollTheDog(dbPoll, now, now)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, ([]ddLog)(nil), logs)
}

func TestPollTheDogPollError(t *testing.T) {
	logger = zaptest.NewLogger(t)

	mock, dbPoll, closeDbFunc := db.SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := dbPoll.NewPoll()
	now := time.Now()
	db.SetupMockPollSelect(mock, poll.Id, now)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	urlTs, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	closeApiClient := setupMockDDogApiClient(urlTs)
	defer closeApiClient()

	logs, err := pollTheDog(dbPoll, now, now)
	assert.EqualError(t, err, "500 Internal Server Error")
	assert.Equal(t, ([]ddLog)(nil), logs)
}

func TestPollTheDogUsePriorPollTime(t *testing.T) {
	logger = zaptest.NewLogger(t)

	mock, dbPoll, closeDbFunc := db.SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := dbPoll.NewPoll()
	now := time.Now()
	db.SetupMockPollSelectAndUpdate(mock, poll.Id, now, 1)

	priorPollTime := now.Add(time.Second * -1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)

		var datadogLogsListRequest datadog.LogsListRequest
		err := json.NewDecoder(r.Body).Decode(&datadogLogsListRequest)
		assert.NoError(t, err)
		assert.Equal(t, priorPollTime.Add(time.Second*pollFudgeSeconds).Format(time.RFC3339), *datadogLogsListRequest.Filter.From)

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	urlTs, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	closeApiClient := setupMockDDogApiClient(urlTs)
	defer closeApiClient()

	logs, err := pollTheDog(dbPoll, priorPollTime, now)
	assert.NoError(t, err)
	assert.Equal(t, ([]ddLog)(nil), logs)
}

func TestPollTheDogOneLog(t *testing.T) {
	logger = zaptest.NewLogger(t)

	mock, dbPoll, closeDbFunc := db.SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := dbPoll.NewPoll()
	now := time.Now()
	db.SetupMockPollSelectAndUpdate(mock, poll.Id, now, 1)

	logId := "myLogId"
	eventSource := "myEventSource"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)

		apiResp := datadog.LogsListResponse{
			Data: &[]datadog.Log{
				{
					Id: &logId,
					Attributes: &datadog.LogAttributes{
						Attributes: map[string]interface{}{
							qryEnv: map[string]interface{}{
								qryEnvExtraJsonFields: map[string]interface{}{
									"eventSource": eventSource,
								},
							},
						},
					},
				},
			},
		}
		jsonObj, err := json.Marshal(apiResp)
		assert.NoError(t, err)
		_, err = w.Write(jsonObj)
		assert.NoError(t, err)
	}))
	defer ts.Close()
	urlTs, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	closeApiClient := setupMockDDogApiClient(urlTs)
	defer closeApiClient()

	logs, err := pollTheDog(dbPoll, now, now)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(logs))
	assert.Equal(t, logId, logs[0].Id)
	assert.Equal(t, eventSource, logs[0].Fields.scoringMessage.EventSource)
}

type MockScoreDB struct {
	t                *testing.T
	assertParameters bool

	selectPriorScore     *types.ParticipantStruct
	selectPriorMsg       *types.ScoringMessage
	selectPriorOldPoints float64

	insertEvtParticipant *types.ParticipantStruct
	insertEvtMsg         *types.ScoringMessage
	insertEvtNewPoints   int
	insertEvtError       error

	updateScoreParticipant *types.ParticipantStruct
	updateScoreDelta       float64
	updateScoreError       error
}

func (m MockScoreDB) GetDb() (db *sql.DB) {
	//TODO implement me
	panic("implement me")
}

func createMockScoreDb(t *testing.T) (scoreDb *MockScoreDB) {
	return &MockScoreDB{
		t:                t,
		assertParameters: true,
	}
}

func (m MockScoreDB) SelectPriorScore(participantToScore *types.ParticipantStruct, msg *types.ScoringMessage) (oldPoints float64) {
	if m.assertParameters {
		assert.Equal(m.t, m.selectPriorScore, participantToScore)
		assert.Equal(m.t, m.selectPriorMsg, msg)
	}
	return m.selectPriorOldPoints
}

func (m MockScoreDB) InsertScoringEvent(participantToScore *types.ParticipantStruct, msg *types.ScoringMessage, newPoints float64) (err error) {
	if m.assertParameters {
		assert.Equal(m.t, m.insertEvtParticipant, participantToScore)
		assert.Equal(m.t, m.insertEvtMsg, msg)
		assert.Equal(m.t, m.insertEvtNewPoints, newPoints)
	}
	return m.insertEvtError
}

func (m MockScoreDB) UpdateParticipantScore(participant *types.ParticipantStruct, delta float64) (err error) {
	if m.assertParameters {
		assert.Equal(m.t, m.updateScoreParticipant, participant)
		assert.Equal(m.t, m.updateScoreDelta, delta)
	}
	return m.updateScoreError
}

var _ db.IScoreDB = (*MockScoreDB)(nil)

func TestProcessLogsZeroLogs(t *testing.T) {
	assert.NoError(t, processLogs(nil, nil, time.Now(), nil))
}

func TestProcessLogsOneWithError(t *testing.T) {
	scoreDb := createMockScoreDb(t)

	logs := []ddLog{
		{},
	}
	now := time.Now()
	forcedError := fmt.Errorf("forced process logs error")
	processScoringMessage := func(scoreDbCalled db.IScoreDB, nowCalled time.Time, msgCalled *types.ScoringMessage) (err error) {
		assert.Equal(t, scoreDb, scoreDbCalled)
		assert.Equal(t, now, nowCalled)
		assert.Equal(t, &types.ScoringMessage{}, msgCalled)
		return forcedError
	}

	err := processLogs(scoreDb, logs, now, processScoringMessage)
	assert.EqualError(t, forcedError, err.Error())
}

func TestProcessLogsOne(t *testing.T) {
	scoreDb := createMockScoreDb(t)

	logs := []ddLog{
		{},
	}
	now := time.Now()
	processScoringMessage := func(scoreDbCalled db.IScoreDB, nowCalled time.Time, msgCalled *types.ScoringMessage) (err error) {
		assert.Equal(t, scoreDb, scoreDbCalled)
		assert.Equal(t, now, nowCalled)
		assert.Equal(t, &types.ScoringMessage{}, msgCalled)
		return
	}

	err := processLogs(scoreDb, logs, now, processScoringMessage)
	assert.NoError(t, err)
}

func TestChaseTailPollError(t *testing.T) {
	logger = zaptest.NewLogger(t)

	mock, dbPoll, closeDbFunc := db.SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := dbPoll.NewPoll()
	forcedError := fmt.Errorf("forced poll db error")
	db.SetupMockPollSelectForcedError(mock, forcedError, poll.Id)

	processScoringMessage := func(scoreDb db.IScoreDB, now time.Time, msg *types.ScoringMessage) (err error) {
		assert.Fail(t, "this should never run")
		return
	}

	quitChan, errChan := ChaseTail(dbPoll, createMockScoreDb(t), 1, processScoringMessage)
	defer close(quitChan)

	assert.EqualError(t, <-errChan, forcedError.Error())
}

func TestChaseTailQuit(t *testing.T) {
	logger = zaptest.NewLogger(t)

	mock, dbPoll, closeDbFunc := db.SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := dbPoll.NewPoll()
	now := time.Now()
	db.SetupMockPollSelectAndUpdateAnyUpdateTime(mock, poll.Id, now, 1)

	processScoringMessage := func(scoreDb db.IScoreDB, now time.Time, msg *types.ScoringMessage) (err error) {
		assert.Fail(t, "this should never run")
		return
	}

	quitChan, errChan := ChaseTail(dbPoll, createMockScoreDb(t), 1, processScoringMessage)
	close(quitChan)
	assert.Nil(t, <-errChan)
}

func TestChaseTailProcessLogsError(t *testing.T) {
	logger = zaptest.NewLogger(t)

	mock, dbPoll, closeDbFunc := db.SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := dbPoll.NewPoll()
	now := time.Now()
	db.SetupMockPollSelectAndUpdateAnyUpdateTime(mock, poll.Id, now, 1)

	logId := "myLogId"
	eventSource := "myEventSource"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)

		apiResp := datadog.LogsListResponse{
			Data: &[]datadog.Log{
				{
					Id: &logId,
					Attributes: &datadog.LogAttributes{
						Attributes: map[string]interface{}{
							qryEnv: map[string]interface{}{
								qryEnvExtraJsonFields: map[string]interface{}{
									"eventSource": eventSource,
								},
							},
						},
					},
				},
			},
		}
		jsonObj, err := json.Marshal(apiResp)
		assert.NoError(t, err)
		_, err = w.Write(jsonObj)
		assert.NoError(t, err)
	}))
	defer ts.Close()
	urlTs, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	closeApiClient := setupMockDDogApiClient(urlTs)
	defer closeApiClient()

	msgProcessed := false
	forcedError := fmt.Errorf("forced process logs error")
	processScoringMessage := func(scoreDb db.IScoreDB, now time.Time, msg *types.ScoringMessage) (err error) {
		msgProcessed = true
		scoreDb.SelectPriorScore(nil, nil)
		assert.NoError(t, scoreDb.UpdateParticipantScore(nil, 0))
		assert.Equal(t, eventSource, msg.EventSource)
		err = forcedError
		return
	}

	quitChan, _ := ChaseTail(dbPoll, createMockScoreDb(t), 1, processScoringMessage)

	time.Sleep(2 * time.Second)
	close(quitChan)
	assert.True(t, msgProcessed)
}

func TestChaseTailOneLog(t *testing.T) {
	logger = zaptest.NewLogger(t)

	mock, dbPoll, closeDbFunc := db.SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := dbPoll.NewPoll()
	now := time.Now()
	db.SetupMockPollSelectAndUpdateAnyUpdateTime(mock, poll.Id, now, 1)

	logId := "myLogId"
	eventSource := "myEventSource"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)

		apiResp := datadog.LogsListResponse{
			Data: &[]datadog.Log{
				{
					Id: &logId,
					Attributes: &datadog.LogAttributes{
						Attributes: map[string]interface{}{
							qryEnv: map[string]interface{}{
								qryEnvExtraJsonFields: map[string]interface{}{
									"eventSource": eventSource,
								},
							},
						},
					},
				},
			},
		}
		jsonObj, err := json.Marshal(apiResp)
		assert.NoError(t, err)
		_, err = w.Write(jsonObj)
		assert.NoError(t, err)
	}))
	defer ts.Close()
	urlTs, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	closeApiClient := setupMockDDogApiClient(urlTs)
	defer closeApiClient()

	msgProcessed := false
	processScoringMessage := func(scoreDb db.IScoreDB, now time.Time, msg *types.ScoringMessage) (err error) {
		msgProcessed = true
		scoreDb.SelectPriorScore(nil, nil)
		assert.NoError(t, scoreDb.UpdateParticipantScore(nil, 0))
		assert.Equal(t, eventSource, msg.EventSource)
		return
	}

	quitChan, _ := ChaseTail(dbPoll, createMockScoreDb(t), 1, processScoringMessage)

	time.Sleep(2 * time.Second)
	close(quitChan)
	assert.True(t, msgProcessed)
}

func TestChaseTailOneLogWithOptMap(t *testing.T) {
	logger = zaptest.NewLogger(t)

	mock, dbPoll, closeDbFunc := db.SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := dbPoll.NewPoll()
	now := time.Now()
	db.SetupMockPollSelectAndUpdateAnyUpdateTime(mock, poll.Id, now, 1)

	eventSource := "myEventSource"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusOK)

		// similar to this:
		// "fixed-bug-types":{"opt":{"semgrep":{"node_password":1,"node_username":1}}}
		mapSemGroupBugType := map[string]interface{}{"sprintf-host-port": float64(2)}
		mapSemGroup := map[string]interface{}{"semgrep": mapSemGroupBugType}
		mapBugTypes := map[string]interface{}{
			"G104":       float64(1),
			"ShellCheck": float64(1),
			"opt":        mapSemGroup,
		}

		logId := "myLogId"
		apiResp := datadog.LogsListResponse{
			Data: &[]datadog.Log{
				{
					Id: &logId,
					Attributes: &datadog.LogAttributes{
						Attributes: map[string]interface{}{
							qryEnv: map[string]interface{}{
								qryEnvExtraJsonFields: map[string]interface{}{
									"eventSource":     eventSource,
									"fixed-bug-types": mapBugTypes,
								},
							},
						},
					},
				},
			},
		}
		jsonObj, err := json.Marshal(apiResp)
		assert.NoError(t, err)
		_, err = w.Write(jsonObj)
		assert.NoError(t, err)
	}))
	defer ts.Close()
	urlTs, err := url.Parse(ts.URL)
	assert.NoError(t, err)

	closeApiClient := setupMockDDogApiClient(urlTs)
	defer closeApiClient()

	msgProcessed := false
	processScoringMessage := func(scoreDb db.IScoreDB, now time.Time, msg *types.ScoringMessage) (err error) {
		msgProcessed = true
		scoreDb.SelectPriorScore(nil, nil)
		assert.NoError(t, scoreDb.UpdateParticipantScore(nil, 0))
		assert.Equal(t, eventSource, msg.EventSource)
		return
	}

	quitChan, _ := ChaseTail(dbPoll, createMockScoreDb(t), 1, processScoringMessage)

	time.Sleep(2 * time.Second)
	close(quitChan)
	assert.True(t, msgProcessed)
}

//goland:noinspection GoUnusedFunction
func xxxTestChaseTailLive(t *testing.T) {
	logger = zaptest.NewLogger(t)

	assert.NoError(t, godotenv.Load("../../.env.dd.bak"))

	mock, dbPoll, closeDbFunc := db.SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := dbPoll.NewPoll()
	now := time.Now()
	// simulate day old poll
	yesterday := now.Add(time.Hour * -24)
	db.SetupMockPollSelectAndUpdateAnyUpdateTime(mock, poll.Id, yesterday, 1)

	processScoringMessage := func(scoreDb db.IScoreDB, now time.Time, msg *types.ScoringMessage) (err error) {
		scoreDb.SelectPriorScore(nil, nil)
		assert.NoError(t, scoreDb.UpdateParticipantScore(nil, 0))
		assert.Equal(t, "github", msg.EventSource)
		return
	}

	quitChan, errChan := ChaseTail(dbPoll, createMockScoreDb(t), 1, processScoringMessage)
	//defer close(quitChan)

	time.Sleep(3 * time.Second)
	close(quitChan)
	assert.Equal(t, nil, <-errChan)
}
