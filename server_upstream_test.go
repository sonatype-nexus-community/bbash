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
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func setupMockWebflowUserUpdate(t *testing.T, webflowId string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", upstreamConfig.participantCollection, webflowId), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
	}))
	return ts
}

func setupMockUpstreamConfig() (resetMockUpstream func()) {
	origUpstreamConfig := upstreamConfig

	upstreamConfig.campaignCollection = campaignUpstreamCollection
	upstreamConfig.token = tokenUpstream
	upstreamConfig.participantCollection = participantUpstreamCollection

	origUpstreamEnabled := upstreamEnabled
	upstreamEnabled = true
	resetMockUpstream = func() {
		upstreamConfig = origUpstreamConfig
		upstreamEnabled = origUpstreamEnabled
	}
	return
}

func TestDoUpstreamRequestWithErrorClientDo(t *testing.T) {
	c, rec := setupMockContextWebflow()
	req, err := http.NewRequest("", "", nil)
	assert.NoError(t, err)
	res, err := doUpstreamRequest(c, req, "")
	assert.EqualError(t, err, "Get \"\": unsupported protocol scheme \"\"")
	assert.Nil(t, res)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestDoUpstreamRequestWithErrorRespStatus(t *testing.T) {
	c, rec := setupMockContextWebflow()
	req, err := http.NewRequest("", "", nil)
	assert.NoError(t, err)
	res, err := doUpstreamRequest(c, req, "")
	assert.EqualError(t, err, "Get \"\": unsupported protocol scheme \"\"")
	assert.Nil(t, res)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestIsCampaignActive(t *testing.T) {
	testCampaign := campaignStruct{StartOn: now.Add(-1 * time.Second), EndOn: now.Add(1 * time.Second)}
	isActive, err := isCampaignActive(testCampaign, now)
	assert.NoError(t, err)
	assert.True(t, isActive)
}

func TestAddCampaignUpstreamAddError(t *testing.T) {
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	testId := "testNewWebflowCampaignId"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", upstreamConfig.campaignCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextCampaign(campaign)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced Scan error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertCampaign)).
		WithArgs(campaign, testId).
		WillReturnError(forcedError)

	expectedError := CreateError{msgPatternCreateErrorCampaign, "500 Internal Server Error"}
	assert.EqualError(t, addCampaign(c), expectedError.Error())
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, expectedError.Error(), rec.Body.String())
}

func TestAddCampaignScanErrorUpstreamEnabled(t *testing.T) {
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	testId := "testNewWebflowCampaignId"
	ts := setupMockWebflowCampaignCreate(t, testId)
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextCampaign(campaign)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced Scan error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertCampaign)).
		WithArgs(campaign, testId, testStartOn, testEndOn).
		WillReturnError(forcedError)

	assert.EqualError(t, addCampaign(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpstreamUpdateScore(t *testing.T) {
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	ts := setupMockWebflowUserUpdate(t, "")
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	upstreamConfig.token = "testWfToken"

	c, rec := setupMockContextUpstreamUpdateScore()

	assert.NoError(t, upstreamUpdateScore(c, "", 0))
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpstreamUpdateScoreStatusError(t *testing.T) {
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/", upstreamConfig.participantCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	upstreamConfig.token = "testWfToken"

	c, rec := setupMockContextUpstreamUpdateScore()

	expectedErr := &ParticipantUpdateError{"404 Not Found"}
	assert.EqualError(t, upstreamUpdateScore(c, "", 0), expectedErr.Error())
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, expectedErr.Error(), rec.Body.String())
}

func TestAddCampaignUpstreamEnabled(t *testing.T) {
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	testId := "testNewWebflowCampaignId"
	ts := setupMockWebflowCampaignCreate(t, testId)
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextCampaign(campaign)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertCampaign)).
		WithArgs(campaign, testId, testStartOn, testEndOn).
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).FromCSVString(campaignId))

	assert.NoError(t, addCampaign(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.Equal(t, campaignId, rec.Body.String())
}

func setupMockWebflowCampaignUpdate(t *testing.T, testId string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", upstreamConfig.campaignCollection, campaignUpstreamId), r.URL.EscapedPath())

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

func TestUpdateCampaignUpstreamEnabled(t *testing.T) {
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	testId := "testNewWebflowCampaignId"
	ts := setupMockWebflowCampaignUpdate(t, testId)
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextCampaign(campaign)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mockUpdateCampaign(mock)

	mockSelectCampaigns(mock)

	assert.NoError(t, updateCampaign(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.Equal(t, campaignId, rec.Body.String())
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

func TestUpstreamNewCampaignWebflowErrorNotFound(t *testing.T) {
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", upstreamConfig.campaignCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextWebflow()

	id, err := upstreamNewCampaign(c, &campaignStruct{}, true)
	assert.Equal(t, "", id)
	expectedErr := &CreateError{msgPatternCreateErrorCampaign, "404 Not Found"}
	assert.EqualError(t, err, expectedErr.Error())
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, expectedErr.Error(), rec.Body.String())
}

func TestUpstreamNewCampaignWebflowIDDecodeError(t *testing.T) {
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", upstreamConfig.campaignCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("bad json text"))
	}))
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextWebflow()

	id, err := upstreamNewCampaign(c, &campaignStruct{}, true)
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
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	testId := "testNewWebflowCampaignId"
	ts := setupMockWebflowCampaignCreate(t, testId)
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextWebflow()

	id, err := upstreamNewCampaign(c, &campaignStruct{}, true)
	assert.Equal(t, testId, id)
	assert.NoError(t, err)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func mockCampaignUpstreamId(mock sqlmock.Sqlmock) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectUpstreamIdCampaign)).
		WithArgs(campaign).
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).AddRow(campaignUpstreamId))
}

func TestGetCampaignUpstreamId(t *testing.T) {
	c, rec := setupMockContext()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()
	mockCampaignUpstreamId(mock)

	id, err := getCampaignUpstreamId(campaign)
	assert.NoError(t, err)
	assert.Equal(t, campaignUpstreamId, id)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestGetCampaignUpstreamIdScanError(t *testing.T) {
	c, rec := setupMockContext()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	forcedError := fmt.Errorf("forced campaign upstream id error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectUpstreamIdCampaign)).
		WithArgs(campaign).
		WillReturnError(forcedError)

	id, err := getCampaignUpstreamId(campaign)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, "", id)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpstreamNewParticipantWebflowErrorNotFound(t *testing.T) {
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	upstreamConfig.participantCollection = "testWfCollection"
	upstreamConfig.token = "testWfToken"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", upstreamConfig.participantCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextWebflow()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()
	mockCampaignUpstreamId(mock)

	id, err := upstreamNewParticipant(c, participant{CampaignName: campaign})
	assert.Equal(t, "", id)
	expectedErr := &CreateError{msgPatternCreateErrorParticipant, "404 Not Found"}
	assert.EqualError(t, err, expectedErr.Error())
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, expectedErr.Error(), rec.Body.String())
}

func TestUpstreamNewParticipantWebflowIDDecodeError(t *testing.T) {
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	upstreamConfig.participantCollection = "testWfCollection"
	upstreamConfig.token = "testWfToken"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", upstreamConfig.participantCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("bad json text"))
	}))
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextWebflow()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()
	mockCampaignUpstreamId(mock)

	id, err := upstreamNewParticipant(c, participant{CampaignName: campaign})
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
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	upstreamConfig.participantCollection = "testWfCollection"
	upstreamConfig.token = "testWfToken"
	testId := "testNewWebflowParticipantId"
	ts := setupMockWebflowUserCreate(t, testId)
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextWebflow()

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()
	mockCampaignUpstreamId(mock)

	id, err := upstreamNewParticipant(c, participant{CampaignName: campaign})
	assert.Equal(t, testId, id)
	assert.NoError(t, err)
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddParticipantWebflowError(t *testing.T) {
	participantJson := fmt.Sprintf(`{"campaignName":"%s", "loginName": "%s"}`, campaign, loginName)
	c, rec := setupMockContextParticipant(participantJson)

	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	upstreamConfig.token = "testWfToken"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items", upstreamConfig.participantCollection), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()
	mockCampaignUpstreamId(mock)

	expectedErr := &CreateError{msgPatternCreateErrorParticipant, "404 Not Found"}
	assert.EqualError(t, addParticipant(c), expectedErr.Error())
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, expectedErr.Error(), rec.Body.String())
}

func TestAddParticipantCampaignMissingUpstreamEnabled(t *testing.T) {
	participantJson := fmt.Sprintf(`{"campaignName":"%s", "loginName": "%s"}`, campaign, loginName)
	c, rec := setupMockContextParticipant(participantJson)

	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	upstreamConfig.token = "testWfToken"
	testId := "testNewWebflowParticipantId"
	ts := setupMockWebflowUserCreate(t, testId)
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()
	mockCampaignUpstreamId(mock)

	forcedError := fmt.Errorf("forced SQL insert error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertParticipant)).
		WillReturnError(forcedError)

	assert.EqualError(t, addParticipant(c), forcedError.Error())
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestAddParticipantUpstreamEnabled(t *testing.T) {
	participantJson := fmt.Sprintf(`{"campaignName":"%s", "scpName": "%s","loginName": "%s"}`, campaign, scpName, loginName)
	c, rec := setupMockContextParticipant(participantJson)

	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	upstreamConfig.token = "testWfToken"
	testId := "testNewWebflowParticipantId"
	ts := setupMockWebflowUserCreate(t, testId)
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()
	mockCampaignUpstreamId(mock)

	participantID := "participantUUId"
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertParticipant)).
		WithArgs(scpName, campaign, loginName, "", "", 0, testId).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "Score", "JoinedAt"}).AddRow(participantID, 0, time.Time{}))

	assert.NoError(t, addParticipant(c))
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `{"guid":"`+participantID+`","endpoints":{"participantDetail"`), rec.Body.String())
	assert.True(t, strings.Contains(rec.Body.String(), `"loginName":"`+loginName+`"`), rec.Body.String())
}

func TestMockWebflow_WithServerUpstreamEnabled(t *testing.T) {
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	upstreamConfig.token = "testWfToken"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	// uncomment 'main()' below for local testing with a mocked Webflow endpoint.
	//main()
}

func TestLogAddParticipantNoErrorUpstreamEnabled(t *testing.T) {
	participantJson := fmt.Sprintf(`{"campaignName":"%s", "scpName": "%s","loginName": "%s"}`, campaign, scpName, loginName)
	c, rec := setupMockContextParticipant(participantJson)

	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	upstreamConfig.token = "testWfToken"
	testId := "testNewWebflowParticipantId"
	ts := setupMockWebflowUserCreate(t, testId)
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()
	mockCampaignUpstreamId(mock)

	participantID := "participantUUId"
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertParticipant)).
		WithArgs(scpName, campaign, loginName, "", "", 0, testId).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "Score", "JoinedAt"}).AddRow(participantID, 0, time.Time{}))

	err := logAddParticipant(c)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusCreated, c.Response().Status)
	assert.True(t, strings.HasPrefix(rec.Body.String(), `{"guid":"`+participantID+`","endpoints":{"participantDetail"`), rec.Body.String())
	assert.True(t, strings.Contains(rec.Body.String(), `"loginName":"`+loginName+`"`), rec.Body.String())
}

func TestUpdateParticipantNoRowsUpdatedUpstreamEnabled(t *testing.T) {
	participantJson := fmt.Sprintf(`{"guid": "%s", "campaignName": "%s", "scpName": "%s", "loginName": "%s", "teamName": "%s"}`, participantID, campaign, scpName, loginName, teamName)
	c, rec := setupMockContextUpdateParticipant(participantJson)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(campaign, scpName, loginName, "", "", 0, teamName, participantID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(0, participantID).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(participantUpstreamId, 0))

	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	ts := setupMockWebflowUserUpdate(t, participantUpstreamId)
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	upstreamConfig.token = "testWfToken"

	assert.NoError(t, updateParticipant(c))
	assert.Equal(t, http.StatusBadRequest, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestUpdateParticipantUpstreamEnabled(t *testing.T) {
	participantJson := fmt.Sprintf(`{"guid": "%s", "campaignName": "%s", "scpName": "%s", "loginName": "%s"}`, participantID, campaign, scpName, loginName)
	c, rec := setupMockContextUpdateParticipant(participantJson)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(campaign, scpName, loginName, "", "", 0, "", participantID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(0, participantID).
		WillReturnRows(sqlmock.NewRows([]string{"UpstreamId", "Score"}).AddRow(participantUpstreamId, 0))

	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	ts := setupMockWebflowUserUpdate(t, participantUpstreamId)
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	upstreamConfig.token = "testWfToken"

	assert.NoError(t, updateParticipant(c))
	assert.Equal(t, http.StatusNoContent, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func setupMockWebflowCampaignDelete(t *testing.T) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", upstreamConfig.participantCollection, participantUpstreamId), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"deleted\": 1}"))
	}))
	return ts
}

func TestDeleteParticipantUpstreamEnabled(t *testing.T) {
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	ts := setupMockWebflowCampaignDelete(t)
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextParticipantDelete(campaign, scpName, loginName)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlDeleteParticipant)).
		WithArgs(campaign, scpName, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"upstreamId"}).AddRow(participantUpstreamId))

	assert.NoError(t, deleteParticipant(c))
	assert.Equal(t, http.StatusOK, c.Response().Status)
	assert.Equal(t, fmt.Sprintf("\"deleted participant: campaign: %s, scpName: %s, loginName: %s, participantUpstreamId: %s\"\n", campaign, scpName, loginName, participantUpstreamId), rec.Body.String())
}

func TestDeleteParticipantWithUpstreamDeleteError(t *testing.T) {
	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", upstreamConfig.participantCollection, participantUpstreamId), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	c, rec := setupMockContextParticipantDelete(campaign, scpName, loginName)

	mock, resetMockDb := setupMockDb(t)
	defer resetMockDb()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlDeleteParticipant)).
		WithArgs(campaign, scpName, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"upstreamId"}).AddRow(participantUpstreamId))

	expectedErr := &CreateError{msgPatternDeleteErrorParticipant, "400 Bad Request"}
	assert.EqualError(t, deleteParticipant(c), expectedErr.Error())
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, expectedErr.Error(), rec.Body.String())
}

func TestNewScoreOneAlertUpdateScoreEndPointErrorNotIgnoredUpstreamEnabled(t *testing.T) {
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

	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", upstreamConfig.participantCollection, strings.ToLower(loginName)), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	upstreamConfig.token = "testWfToken"

	err = newScore(c)
	assert.EqualError(t, err, "could not update score. response status: 400 Bad Request")
	assert.Equal(t, http.StatusInternalServerError, c.Response().Status)
	assert.Equal(t, "could not update score. response status: 400 Bad Request", rec.Body.String())
}

func TestNewScoreOneAlertCommitErrorNotIgnoredUpstreamEnabled(t *testing.T) {
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

	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", upstreamConfig.participantCollection, strings.ToLower(loginName)), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	upstreamConfig.token = "testWfToken"

	err = newScore(c)
	assert.EqualError(t, err, "all expectations were already fulfilled, call to Commit transaction was not expected")
	assert.Equal(t, 0, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertUserCapitalizationMismatchUpstreamEnabled(t *testing.T) {
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

	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", upstreamConfig.participantCollection, loginNameLowerCase), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	upstreamConfig.token = "testWfToken"

	mock.ExpectCommit()

	err = newScore(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}

func TestNewScoreOneAlertUpstreamEnabled(t *testing.T) {
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

	resetMockUpstream := setupMockUpstreamConfig()
	defer resetMockUpstream()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, fmt.Sprintf("/collections/%s/items/%s", upstreamConfig.participantCollection, loginName), r.URL.EscapedPath())

		verifyRequestHeaders(t, r)

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	upstreamConfig.baseAPI = ts.URL

	upstreamConfig.token = "testWfToken"

	mock.ExpectCommit()

	err = newScore(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, c.Response().Status)
	assert.Equal(t, "", rec.Body.String())
}
