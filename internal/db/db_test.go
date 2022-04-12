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

package db

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/sonatype-nexus-community/bbash/internal/types"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestConvertSqlToDbMockExpect(t *testing.T) {
	// sanity check all the cases we've found so far
	assert.Equal(t, `\$\(\)\*\+`, convertSqlToDbMockExpect(`$()*+`))
}

// exclude parent 'db' directory for tests
const testMigrateSourceURL = "file://migrations/v2"

func TestMigrateDBErrorPostgresWithInstance(t *testing.T) {
	_, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	assert.EqualError(t, db.MigrateDB(testMigrateSourceURL), "all expectations were already fulfilled, call to Query 'SELECT CURRENT_DATABASE()' with args [] was not expected in line 0: SELECT CURRENT_DATABASE()")
}

func TestMigrateDBErrorMigrateUp(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	// mocks for 'postgres.WithInstance()'
	mock.ExpectQuery(`SELECT CURRENT_DATABASE()`).
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).FromCSVString("theDatabaseName"))
	mock.ExpectQuery(`SELECT CURRENT_SCHEMA()`).
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).FromCSVString("theDatabaseSchema"))

	// this value may change when version of migrate is upgraded
	args := []driver.Value{"1560208929"}
	mock.ExpectExec(convertSqlToDbMockExpect(`SELECT pg_advisory_lock($1)`)).
		WithArgs(args...).
		//WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery(convertSqlToDbMockExpect(`SELECT COUNT(1) FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2 LIMIT 1`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectExec(convertSqlToDbMockExpect(`CREATE TABLE IF NOT EXISTS "theDatabaseSchema"."schema_migrations" (version bigint not null primary key, dirty boolean not null)`)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectExec(convertSqlToDbMockExpect(`SELECT pg_advisory_unlock($1)`)).
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 0))

	assert.EqualError(t, db.MigrateDB(testMigrateSourceURL), fmt.Sprintf("try lock failed in line 0: SELECT pg_advisory_lock($1) (details: all expectations were already fulfilled, call to ExecQuery 'SELECT pg_advisory_lock($1)' with args [{Name: Ordinal:1 Value:%s}] was not expected)", args[0]))
}

func TestGetSourceControlProvidersQueryError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced scp error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectSourceControlProvider)).
		WillReturnError(forcedError)

	scps, err := db.GetSourceControlProviders()
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, ([]types.SourceControlProviderStruct)(nil), scps)
}

func TestGetSourceControlProvidersScanError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectSourceControlProvider)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "name", "url"}).
			// force scan error via invalid datatype
			AddRow("someId", "someSCP", sql.NullString{}))

	scps, err := db.GetSourceControlProviders()
	assert.EqualError(t, err, "sql: Scan error on column index 2, name \"url\": converting NULL to string is unsupported")
	assert.Equal(t, ([]types.SourceControlProviderStruct)(nil), scps)
}

func TestGetSourceControlProviders(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectSourceControlProvider)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "name", "url"}).AddRow("someId", "someSCP", "someUrl"))

	scps, err := db.GetSourceControlProviders()
	assert.NoError(t, err)
	assert.Equal(t, []types.SourceControlProviderStruct{
		{"someId", "someSCP", "someUrl"},
	}, scps)
}

var campaignStartTime = time.Now()
var campaignEndTime = campaignStartTime.Add(time.Second)
var testCampaign = types.CampaignStruct{
	Name:    "testCampaignName",
	StartOn: campaignStartTime,
	EndOn:   campaignEndTime,
}

const testCampaignGuid = "testCampaignGuid"

const testOrganizationGuid = "testOrganizationGuid"

var testOrganization = types.OrganizationStruct{
	ID:           testOrganizationGuid,
	SCPName:      "scpName",
	Organization: "myOrganizationName",
}

const testBugType = "testBugType"

func TestInsertCampaignError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced SQL insert error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertCampaign)).
		WithArgs(testCampaign.Name, testCampaign.StartOn, testCampaign.EndOn).
		WillReturnError(forcedError)

	guid, err := db.InsertCampaign(&testCampaign)
	assert.Error(t, err, forcedError.Error())
	assert.Equal(t, "", guid)
}

func TestInsertCampaign(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertCampaign)).
		WithArgs(testCampaign.Name, testCampaign.StartOn, testCampaign.EndOn).
		WillReturnRows(sqlmock.NewRows([]string{"guid"}).AddRow(testCampaignGuid))

	guid, err := db.InsertCampaign(&testCampaign)
	assert.NoError(t, err)
	assert.Equal(t, testCampaignGuid, guid)
}

func TestUpdateCampaignError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced SQL insert error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateCampaign)).
		WithArgs(testCampaign.Name, testCampaign.StartOn, testCampaign.EndOn).
		WillReturnError(forcedError)

	guid, err := db.UpdateCampaign(&testCampaign)
	assert.Error(t, err, forcedError.Error())
	assert.Equal(t, "", guid)
}

func TestUpdateCampaign(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateCampaign)).
		WithArgs(testCampaign.StartOn, testCampaign.EndOn, testCampaign.Name).
		WillReturnRows(sqlmock.NewRows([]string{"guid"}).AddRow(testCampaignGuid))

	guid, err := db.UpdateCampaign(&testCampaign)
	assert.NoError(t, err)
	assert.Equal(t, testCampaignGuid, guid)
}

func TestGetCampaignError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced campaign error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaign)).
		WillReturnError(forcedError)

	campaign, err := db.GetCampaign(testCampaign.Name)
	assert.Error(t, err, forcedError.Error())
	assert.Nil(t, campaign)
}

func TestGetCampaignScanError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaign)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "startOn", "endOn", "note"}).
			// force scan error due to time.Time type mismatch at CreatedOn column
			AddRow("campaignId", "campaignName", "badness", 1, time.Time{}, time.Time{}, ""))

	campaign, err := db.GetCampaign(testCampaign.Name)
	assert.EqualError(t, err, `sql: Scan error on column index 2, name "createdOn": unsupported Scan, storing driver.Value type string into type *time.Time`)
	assert.Equal(t, "campaignId", campaign.ID)
}

func TestGetCampaign(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaign)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "startOn", "endOn", "note"}).
			AddRow(testCampaign.ID, testCampaign.Name, testCampaign.CreatedOn, testCampaign.CreatedOrder, testCampaign.StartOn, testCampaign.EndOn, testCampaign.Note))

	campaign, err := db.GetCampaign(testCampaign.Name)
	assert.NoError(t, err)
	assert.Equal(t, &testCampaign, campaign)
}

func TestGetCampaignsError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced campaign error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaigns)).
		WillReturnError(forcedError)

	campaigns, err := db.GetCampaigns()
	assert.Error(t, err, forcedError.Error())
	assert.Nil(t, campaigns)
}

func TestGetCampaignsScanError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaigns)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "startOn", "endOn", "note"}).
			// force scan error due to time.Time type mismatch at CreatedOn column
			AddRow("campaignId", "campaignName", "badness", 1, time.Time{}, time.Time{}, ""))

	campaigns, err := db.GetCampaigns()
	assert.EqualError(t, err, `sql: Scan error on column index 2, name "createdOn": unsupported Scan, storing driver.Value type string into type *time.Time`)
	assert.Nil(t, campaigns)
}

func TestGetCampaigns(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCampaigns)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "startOn", "endOn", "note"}).
			AddRow(testCampaign.ID, testCampaign.Name, testCampaign.CreatedOn, testCampaign.CreatedOrder, testCampaign.StartOn, testCampaign.EndOn, testCampaign.Note))

	campaigns, err := db.GetCampaigns()
	assert.NoError(t, err)
	assert.Equal(t, []types.CampaignStruct{testCampaign}, campaigns)
}

var now = time.Now()

func TestGetActiveCampaignsError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced campaign error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCurrentCampaigns)).
		WillReturnError(forcedError)

	activeCampaigns, err := db.GetActiveCampaigns(now)
	assert.EqualError(t, err, forcedError.Error())
	var expectedCampaigns []types.CampaignStruct
	assert.Equal(t, expectedCampaigns, activeCampaigns)
}

func TestGetActiveCampaignsScanError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCurrentCampaigns)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "startOn", "endOn", "note"}).
			// force scan error due to time.Time type mismatch at CreatedOn column
			AddRow("campaignId", "campaignName", "badness", 0, now, now, sql.NullString{}))

	activeCampaigns, err := db.GetActiveCampaigns(now)
	assert.EqualError(t, err, `sql: Scan error on column index 2, name "createdOn": unsupported Scan, storing driver.Value type string into type *time.Time`)
	var expectedCampaigns []types.CampaignStruct
	assert.Equal(t, expectedCampaigns, activeCampaigns)
}

func TestGetActiveCampaigns(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectCurrentCampaigns)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "createdOn", "createOrder", "startOn", "endOn", "note"}).
			AddRow(testCampaign.ID, testCampaign.Name, time.Time{}, 0, now, now, sql.NullString{}))

	activeCampaigns, err := db.GetActiveCampaigns(now)
	assert.NoError(t, err)
	expectedCampaigns := []types.CampaignStruct{
		{ID: testCampaign.ID, Name: testCampaign.Name, StartOn: now, EndOn: now},
	}
	assert.Equal(t, expectedCampaigns, activeCampaigns)
}

func TestInsertOrganizationInsertError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced org add error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertOrganization)).
		WillReturnError(forcedError)

	guid, err := db.InsertOrganization(&types.OrganizationStruct{})
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, "", guid)
}

func TestAddOrganization(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertOrganization)).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).AddRow("someId"))

	guid, err := db.InsertOrganization(&testOrganization)
	assert.NoError(t, err)
	assert.Equal(t, "someId", guid)
}

func TestGetOrganizationsError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedErr := fmt.Errorf("forced org list error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizations)).
		WillReturnError(forcedErr)

	organizations, err := db.GetOrganizations()
	assert.EqualError(t, err, forcedErr.Error())
	assert.Nil(t, organizations)
}

func TestGetOrganizationsScanError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizations)).
		WillReturnRows(sqlmock.NewRows([]string{}).AddRow())

	organizations, err := db.GetOrganizations()
	assert.EqualError(t, err, "sql: expected 0 destination arguments in Scan, not 3")
	assert.Nil(t, organizations)
}

func TestGetOrganizations(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizations)).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "SCPName", "Org"}).
			AddRow(testOrganization.ID, testOrganization.SCPName, testOrganization.Organization))

	organizations, err := db.GetOrganizations()
	assert.NoError(t, err)
	assert.Equal(t, organizations, []types.OrganizationStruct{testOrganization})
}

func TestDeleteOrganizationDeleteError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced org delete error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlDeleteOrganization)).
		WillReturnError(forcedError)

	rowsAffected, err := db.DeleteOrganization("", "")
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, int64(0), rowsAffected)
}

func TestDeleteOrganizationNotFound(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectExec(convertSqlToDbMockExpect(sqlDeleteOrganization)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	rowsAffected, err := db.DeleteOrganization("", "")
	assert.NoError(t, err)
	assert.Equal(t, int64(0), rowsAffected)
}

func TestDeleteOrganization(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectExec(convertSqlToDbMockExpect(sqlDeleteOrganization)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	rowsAffected, err := db.DeleteOrganization("", "")
	assert.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

func TestValidOrganizationFalse(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs(TestEventSourceValid, TestOrgValid).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	msg := &types.ScoringMessage{EventSource: TestEventSourceValid, RepoOwner: TestOrgValid}
	isValidOrg, err := db.ValidOrganization(msg)
	assert.Nil(t, err)
	assert.False(t, isValidOrg)
}

func TestValidOrganizationError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced org exists query error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs("GitHub", TestOrgValid).
		WillReturnError(forcedError)

	msg := &types.ScoringMessage{EventSource: "GitHub", RepoOwner: TestOrgValid}
	isValidOrg, err := db.ValidOrganization(msg)
	assert.EqualError(t, err, forcedError.Error())
	assert.False(t, isValidOrg)
}

func TestValidOrganization(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectOrganizationExists)).
		WithArgs(TestEventSourceValid, TestOrgValid).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	msg := &types.ScoringMessage{EventSource: TestEventSourceValid, RepoOwner: TestOrgValid}
	isValidOrg, err := db.ValidOrganization(msg)
	assert.Nil(t, err)
	assert.True(t, isValidOrg)
}

const loginName = "loginName"

func TestSelectParticipantsToScoreSelectError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced current campaign read error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(now, TestEventSourceValid, loginName).
		WillReturnError(forcedError)

	msg := &types.ScoringMessage{EventSource: TestEventSourceValid, RepoOwner: TestOrgValid, TriggerUser: loginName}

	participantsToScore, err := db.SelectParticipantsToScore(msg, now)
	assert.EqualError(t, err, forcedError.Error())
	assert.Nil(t, participantsToScore)
}

func TestSelectParticipantsToScoreScanError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(now, TestEventSourceValid, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"Id"}).
			// force scan error due to mismatched column count
			AddRow(-1))

	msg := &types.ScoringMessage{EventSource: TestEventSourceValid, RepoOwner: TestOrgValid, TriggerUser: loginName}

	participantsToScore, err := db.SelectParticipantsToScore(msg, now)
	assert.EqualError(t, err, "sql: expected 1 destination arguments in Scan, not 5")
	assert.Nil(t, participantsToScore)
}

func TestSelectParticipantsToScoreValidTeam(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(now, TestEventSourceValid, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignName", "SCPName", "loginName", "teamName"}).
			// force scan error due to type mismatch at ID column
			AddRow(now, "someCampaign", "someSCP", "someLoginName", "someTeamName"))

	msg := &types.ScoringMessage{EventSource: TestEventSourceValid, RepoOwner: TestOrgValid, TriggerUser: loginName}

	participantsToScore, err := db.SelectParticipantsToScore(msg, now)
	assert.NoError(t, err)
	assert.Equal(t, "someTeamName", participantsToScore[0].TeamName)
}

func TestSelectParticipantsToScoreNoTeam(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantId)).
		WithArgs(now, TestEventSourceValid, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"Id", "CampaignName", "SCPName", "loginName", "teamName"}).
			// force scan error due to type mismatch at ID column
			AddRow(now, "someCampaign", "someSCP", "someLoginName", nil))

	msg := &types.ScoringMessage{EventSource: TestEventSourceValid, RepoOwner: TestOrgValid, TriggerUser: loginName}

	participantsToScore, err := db.SelectParticipantsToScore(msg, now)
	assert.NoError(t, err)
	assert.Equal(t, "", participantsToScore[0].TeamName)
}

func TestSelectPointValueScanError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced point value error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectPointValue)).
		WithArgs(testCampaign.Name, testBugType).
		WillReturnError(forcedError)

	msg := &types.ScoringMessage{EventSource: TestEventSourceValid, RepoOwner: TestOrgValid, TriggerUser: loginName}

	participantsToScore := db.SelectPointValue(msg, testCampaign.Name, testBugType)
	assert.Equal(t, float64(1), participantsToScore)
}

func TestSelectPointValueRead(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectPointValue)).
		WithArgs(testCampaign.Name, testBugType).
		WillReturnRows(sqlmock.NewRows([]string{"points"}).AddRow(5))

	msg := &types.ScoringMessage{EventSource: TestEventSourceValid, RepoOwner: TestOrgValid, TriggerUser: loginName}

	participantsToScore := db.SelectPointValue(msg, testCampaign.Name, testBugType)
	assert.Equal(t, float64(5), participantsToScore)
}

const testParticipantGuid = "testParticipantGuid"

func TestUpdateParticipantScoreError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced update score error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(float64(0), testParticipantGuid).
		WillReturnError(forcedError)

	err := db.UpdateParticipantScore(&types.ParticipantStruct{ID: testParticipantGuid}, 0)
	assert.EqualError(t, err, forcedError.Error())
}

func TestUpdateParticipantScoreZero(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlUpdateParticipantScore)).
		WithArgs(float64(0), testParticipantGuid).
		WillReturnRows(sqlmock.NewRows([]string{"score"}).AddRow(3))

	assert.NoError(t, db.UpdateParticipantScore(&types.ParticipantStruct{ID: testParticipantGuid}, 0))
}

func TestSelectPriorScoreError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	testParticipant := &types.ParticipantStruct{
		ID:           testParticipantGuid,
		CampaignName: testCampaign.Name,
		ScpName:      "scpName",
	}

	msg := &types.ScoringMessage{RepoOwner: TestOrgValid, RepoName: "testRepoName", TriggerUser: loginName, PullRequest: -1}

	forcedError := fmt.Errorf("forced prior score error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(testParticipant.CampaignName, testParticipant.ScpName, msg.RepoOwner, msg.RepoName, msg.PullRequest).
		WillReturnError(forcedError)

	oldPoints := db.SelectPriorScore(testParticipant, msg)
	assert.Equal(t, float64(0), oldPoints)
}

func TestSelectPriorScore(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	testParticipant := &types.ParticipantStruct{
		ID:           testParticipantGuid,
		CampaignName: testCampaign.Name,
		ScpName:      "scpName",
	}

	msg := &types.ScoringMessage{RepoOwner: TestOrgValid, RepoName: "testRepoName", TriggerUser: loginName, PullRequest: -1}

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlScoreQuery)).
		WithArgs(testParticipant.CampaignName, testParticipant.ScpName, msg.RepoOwner, msg.RepoName, msg.PullRequest).
		WillReturnRows(sqlmock.NewRows([]string{"score"}).AddRow(-2))

	oldPoints := db.SelectPriorScore(testParticipant, msg)
	assert.Equal(t, float64(-2), oldPoints)
}

func TestInsertScoringEventError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	testParticipant := &types.ParticipantStruct{
		ID:           testParticipantGuid,
		CampaignName: testCampaign.Name,
		ScpName:      "scpName",
	}

	msg := &types.ScoringMessage{RepoOwner: TestOrgValid, RepoName: "testRepoName", TriggerUser: loginName, PullRequest: -1}

	const newPoints = float64(11)

	forcedError := fmt.Errorf("forced insert score error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlInsertScoringEvent)).
		WithArgs(testParticipant.CampaignName, testParticipant.ScpName, msg.RepoOwner, msg.RepoName, msg.PullRequest, msg.TriggerUser, newPoints).
		WillReturnError(forcedError)

	assert.EqualError(t, db.InsertScoringEvent(testParticipant, msg, newPoints), forcedError.Error())
}

func TestInsertScoringEvent(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	testParticipant := &types.ParticipantStruct{
		ID:           testParticipantGuid,
		CampaignName: testCampaign.Name,
		ScpName:      "scpName",
	}

	msg := &types.ScoringMessage{RepoOwner: TestOrgValid, RepoName: "testRepoName", TriggerUser: loginName, PullRequest: -1}

	const newPoints = float64(11)

	mock.ExpectExec(convertSqlToDbMockExpect(sqlInsertScoringEvent)).
		WithArgs(testParticipant.CampaignName, testParticipant.ScpName, msg.RepoOwner, msg.RepoName, msg.PullRequest, msg.TriggerUser, newPoints).
		WillReturnResult(sqlmock.NewResult(0, 1))

	assert.NoError(t, db.InsertScoringEvent(testParticipant, msg, newPoints))
}

func TestInsertParticipantError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	testParticipant := types.ParticipantStruct{
		Score: -2,
	}

	forcedError := fmt.Errorf("forced insert participant error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertParticipant)).
		WithArgs(testParticipant.ScpName, testParticipant.CampaignName,
			testParticipant.LoginName, testParticipant.Email, testParticipant.DisplayName, 0).
		WillReturnError(forcedError)

	assert.EqualError(t, db.InsertParticipant(&testParticipant), forcedError.Error())
	assert.Equal(t, "", testParticipant.ID)
	assert.Equal(t, -2, testParticipant.Score)
	assert.Equal(t, time.Time{}, testParticipant.JoinedAt)
}

func TestInsertParticipant(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	testParticipant := types.ParticipantStruct{
		// ID will be empty when created from endpoint request
		CampaignName: testCampaign.Name,
		ScpName:      "scpName",
		LoginName:    "loginName",
		Email:        "email",
		DisplayName:  "displayName",
		Score:        -1, // this should be ignored during insert
	}

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertParticipant)).
		WithArgs(testParticipant.ScpName, testParticipant.CampaignName,
			testParticipant.LoginName, testParticipant.Email, testParticipant.DisplayName, 0).
		WillReturnRows(sqlmock.NewRows([]string{"guid", "score", "joinedAt"}).
			AddRow(testParticipantGuid, 0, now))

	assert.NoError(t, db.InsertParticipant(&testParticipant))
	assert.Equal(t, testParticipantGuid, testParticipant.ID)
	assert.Equal(t, 0, testParticipant.Score)
	assert.Equal(t, now, testParticipant.JoinedAt)
}

func TestInsertTeamError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	testTeam := types.TeamStruct{
		// ID will be empty when created from endpoint request
		CampaignName: testCampaign.Name,
		Name:         "teamName",
	}

	forcedError := fmt.Errorf("forced insert team error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertTeam)).
		WithArgs(testTeam.CampaignName, testTeam.Name).
		WillReturnError(forcedError)

	assert.EqualError(t, db.InsertTeam(&testTeam), forcedError.Error())
	assert.Equal(t, "", testTeam.Id)
}

const testTeamGuid = "testTeamGuid"

func TestInsertTeam(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	testTeam := types.TeamStruct{
		// ID will be empty when created from endpoint request
		CampaignName: testCampaign.Name,
		Name:         "teamName",
	}

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertTeam)).
		WithArgs(testTeam.CampaignName, testTeam.Name).
		WillReturnRows(sqlmock.NewRows([]string{"guid"}).
			AddRow(testTeamGuid))

	err := db.InsertTeam(&testTeam)
	assert.NoError(t, err)
	assert.Equal(t, testTeamGuid, testTeam.Id)
}

const campaignName = "campaignName"
const scpName = "scpName"

func TestSelectParticipantDetailError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced insert team error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantDetail)).
		WithArgs(campaignName, scpName, loginName).
		WillReturnError(forcedError)

	participant, err := db.SelectParticipantDetail(campaignName, scpName, loginName)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, &types.ParticipantStruct{}, participant)
}

func TestSelectParticipantDetailNoTeam(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantDetail)).
		WithArgs(campaignName, scpName, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"guid", "campaign", "scp", "login", "email", "display", "score", "team", "joinedAt"}).
			AddRow(testParticipantGuid, campaignName, scpName, loginName, "email", "display", -1, sql.NullString{}, now))

	participant, err := db.SelectParticipantDetail(campaignName, scpName, loginName)
	assert.NoError(t, err)
	assert.Equal(t, &types.ParticipantStruct{
		ID:           testParticipantGuid,
		CampaignName: campaignName,
		ScpName:      scpName,
		LoginName:    loginName,
		Email:        "email",
		DisplayName:  "display",
		Score:        -1,
		TeamName:     "",
		JoinedAt:     now,
	}, participant)
}

func TestSelectParticipantDetail(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	const campaignName = "campaignName"
	const scpName = "scpName"
	const loginName = "loginName"

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantDetail)).
		WithArgs(campaignName, scpName, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"guid", "campaign", "scp", "login", "email", "display", "score", "team", "joinedAt"}).
			AddRow(testParticipantGuid, campaignName, scpName, loginName, "email", "display", -1, "teamName", now))

	participant, err := db.SelectParticipantDetail(campaignName, scpName, loginName)
	assert.NoError(t, err)
	assert.Equal(t, &types.ParticipantStruct{
		ID:           testParticipantGuid,
		CampaignName: campaignName,
		ScpName:      scpName,
		LoginName:    loginName,
		Email:        "email",
		DisplayName:  "display",
		Score:        -1,
		TeamName:     "teamName",
		JoinedAt:     now,
	}, participant)
}

func TestSelectParticipantsInCampaignError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced select campaign participants error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantsByCampaign)).
		WithArgs(campaignName).
		WillReturnError(forcedError)

	participants, err := db.SelectParticipantsInCampaign(campaignName)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, ([]types.ParticipantStruct)(nil), participants)
}

func TestSelectParticipantsInCampaignScanError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantsByCampaign)).
		WithArgs(campaignName).
		WillReturnRows(sqlmock.NewRows([]string{"guid", "campaign", "scp", "login", "email", "display", "score", "team", "joinedAt"}).
			// force scan error with nil in JoinedAt Time field
			AddRow(testParticipantGuid, campaignName, scpName, loginName, "email", "display", -1, "teamName", nil))

	participants, err := db.SelectParticipantsInCampaign(campaignName)
	assert.EqualError(t, err, "sql: Scan error on column index 8, name \"joinedAt\": unsupported Scan, storing driver.Value type <nil> into type *time.Time")
	assert.Equal(t, ([]types.ParticipantStruct)(nil), participants)
}

func TestSelectParticipantsInCampaignNoTeam(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantsByCampaign)).
		WithArgs(campaignName).
		WillReturnRows(sqlmock.NewRows([]string{"guid", "campaign", "scp", "login", "email", "display", "score", "team", "joinedAt"}).
			AddRow(testParticipantGuid, campaignName, scpName, loginName, "email", "display", -1, sql.NullString{}, now))

	participants, err := db.SelectParticipantsInCampaign(campaignName)
	assert.NoError(t, err)
	assert.Equal(t, []types.ParticipantStruct{
		{
			ID:           testParticipantGuid,
			CampaignName: campaignName,
			ScpName:      scpName,
			LoginName:    loginName,
			Email:        "email",
			DisplayName:  "display",
			Score:        -1,
			TeamName:     "",
			JoinedAt:     now,
		},
	}, participants)
}

func TestSelectParticipantsInCampaign(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectParticipantsByCampaign)).
		WithArgs(campaignName).
		WillReturnRows(sqlmock.NewRows([]string{"guid", "campaign", "scp", "login", "email", "display", "score", "team", "joinedAt"}).
			AddRow(testParticipantGuid, campaignName, scpName, loginName, "email", "display", -1, "teamName", now))

	participants, err := db.SelectParticipantsInCampaign(campaignName)
	assert.NoError(t, err)
	assert.Equal(t, []types.ParticipantStruct{
		{
			ID:           testParticipantGuid,
			CampaignName: campaignName,
			ScpName:      scpName,
			LoginName:    loginName,
			Email:        "email",
			DisplayName:  "display",
			Score:        -1,
			TeamName:     "teamName",
			JoinedAt:     now,
		},
	}, participants)
}

func TestUpdateParticipantError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	testParticipant := types.ParticipantStruct{}
	forcedError := fmt.Errorf("forced update participant error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(testParticipant.CampaignName, testParticipant.ScpName, testParticipant.LoginName, testParticipant.Email,
			testParticipant.DisplayName, testParticipant.Score, testParticipant.TeamName,
			testParticipant.ID).
		WillReturnError(forcedError)

	rowsAffected, err := db.UpdateParticipant(&testParticipant)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, int64(0), rowsAffected)
}

func TestUpdateParticipantRowsAffectedError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	testParticipant := types.ParticipantStruct{
		ID:           testParticipantGuid,
		CampaignName: campaignName,
		ScpName:      scpName,
		LoginName:    loginName,
		Email:        "email",
		DisplayName:  "display",
		Score:        -1,
		TeamName:     "teamName",
		JoinedAt:     now,
	}
	forcedError := fmt.Errorf("forced update participant rows affected error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(testParticipant.CampaignName, testParticipant.ScpName, testParticipant.LoginName, testParticipant.Email,
			testParticipant.DisplayName, testParticipant.Score, testParticipant.TeamName,
			testParticipant.ID).
		WillReturnResult(sqlmock.NewErrorResult(forcedError))

	rowsAffected, err := db.UpdateParticipant(&testParticipant)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, int64(0), rowsAffected)
}

func TestUpdateParticipant(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	testParticipant := types.ParticipantStruct{
		ID:           testParticipantGuid,
		CampaignName: campaignName,
		ScpName:      scpName,
		LoginName:    loginName,
		Email:        "email",
		DisplayName:  "display",
		Score:        -1,
		TeamName:     "teamName",
		JoinedAt:     now,
	}
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipant)).
		WithArgs(testParticipant.CampaignName, testParticipant.ScpName, testParticipant.LoginName, testParticipant.Email,
			testParticipant.DisplayName, testParticipant.Score, testParticipant.TeamName,
			testParticipant.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	rowsAffected, err := db.UpdateParticipant(&testParticipant)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

func TestDeleteParticipantError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced delete participant error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlDeleteParticipant)).
		WithArgs(campaignName, scpName, loginName).
		WillReturnError(forcedError)

	deletedParticipantId, err := db.DeleteParticipant(campaignName, scpName, loginName)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, "", deletedParticipantId)
}

func TestDeleteParticipant(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlDeleteParticipant)).
		WithArgs(campaignName, scpName, loginName).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(testParticipantGuid))

	deletedParticipantId, err := db.DeleteParticipant(campaignName, scpName, loginName)
	assert.NoError(t, err)
	assert.Equal(t, testParticipantGuid, deletedParticipantId)
}

const teamName = "teamName"

func TestUpdateParticipantTeamError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced update participant team error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipantTeam)).
		WithArgs(teamName, campaignName, scpName, loginName).
		WillReturnError(forcedError)

	rowsAffected, err := db.UpdateParticipantTeam(teamName, campaignName, scpName, loginName)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, int64(0), rowsAffected)
}

func TestUpdateParticipantTeamRowsAffectedError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced update participant team rows affected error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipantTeam)).
		WithArgs(teamName, campaignName, scpName, loginName).
		WillReturnResult(sqlmock.NewErrorResult(forcedError))

	rowsAffected, err := db.UpdateParticipantTeam(teamName, campaignName, scpName, loginName)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, int64(0), rowsAffected)
}

func TestUpdateParticipantTeam(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateParticipantTeam)).
		WithArgs(teamName, campaignName, scpName, loginName).
		WillReturnResult(sqlmock.NewResult(0, 1))

	rowsAffected, err := db.UpdateParticipantTeam(teamName, campaignName, scpName, loginName)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

const bugCategory = "bugCategory"
const bugGuid = "bugGuid"

func TestInsertBugError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	bug := types.BugStruct{
		// empty Id before insert
		Campaign:   campaignName,
		Category:   bugCategory,
		PointValue: 2,
	}
	forcedError := fmt.Errorf("forced insert bug error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertBug)).
		WithArgs(bug.Campaign, bug.Category, bug.PointValue).
		WillReturnError(forcedError)

	assert.EqualError(t, db.InsertBug(&bug), forcedError.Error())
	assert.Equal(t, "", bug.Id)
}

func TestInsertBug(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	bug := types.BugStruct{
		// empty Id before insert
		Campaign:   campaignName,
		Category:   bugCategory,
		PointValue: 2,
	}
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlInsertBug)).
		WithArgs(bug.Campaign, bug.Category, bug.PointValue).
		WillReturnRows(sqlmock.NewRows([]string{"guid"}).AddRow(bugGuid))

	assert.NoError(t, db.InsertBug(&bug))
	assert.Equal(t, bugGuid, bug.Id)
}

func TestUpdateBugError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	bug := types.BugStruct{}
	forcedError := fmt.Errorf("forced update bug error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateBug)).
		WithArgs(bug.PointValue, bug.Campaign, bug.Category).
		WillReturnError(forcedError)

	rowsAffected, err := db.UpdateBug(&bug)
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, int64(0), rowsAffected)
}

func TestUpdateBug(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	bug := types.BugStruct{
		// empty Id before insert
		Campaign:   campaignName,
		Category:   bugCategory,
		PointValue: 5,
	}
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdateBug)).
		WithArgs(bug.PointValue, bug.Campaign, bug.Category).
		WillReturnResult(sqlmock.NewResult(0, 1))

	rowsAffected, err := db.UpdateBug(&bug)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

func TestSelectBugsError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	forcedError := fmt.Errorf("forced select bugs error")
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectBugs)).
		WillReturnError(forcedError)

	bugs, err := db.SelectBugs()
	assert.EqualError(t, err, forcedError.Error())
	assert.Equal(t, ([]types.BugStruct)(nil), bugs)
}

func TestSelectBugsScanError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectBugs)).
		// force scan error with invalid column count
		WillReturnRows(sqlmock.NewRows([]string{"badColumn"}).AddRow(-1))

	bugs, err := db.SelectBugs()
	assert.EqualError(t, err, "sql: expected 1 destination arguments in Scan, not 4")
	assert.Equal(t, ([]types.BugStruct)(nil), bugs)
}

func TestSelectBugs(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	bug := types.BugStruct{
		// empty Id before insert
		Campaign:   campaignName,
		Category:   bugCategory,
		PointValue: 5,
	}
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectBugs)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "campiagn", "category", "pointValue"}).
			AddRow(bug.Id, bug.Campaign, bug.Category, bug.PointValue))

	bugs, err := db.SelectBugs()
	assert.NoError(t, err)
	assert.Equal(t, []types.BugStruct{bug}, bugs)
}

func TestGetDb(t *testing.T) {
	_, dbFake, closeDbFunc := SetupMockDB(t)
	defer closeDbFunc()

	assert.NotNil(t, dbFake.GetDb())
	assert.NotNil(t, dbFake.logger)
}
