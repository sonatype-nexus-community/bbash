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
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/sonatype-nexus-community/bbash/internal/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
	"strings"
	"testing"
	"time"
)

func TestGetLogger(t *testing.T) {
	testLogger := zaptest.NewLogger(t)
	pollDb := NewDBPoll(nil, testLogger)
	assert.NotNil(t, pollDb)
	assert.Equal(t, testLogger, pollDb.GetLogger())
}

func TestNewPoll(t *testing.T) {
	_, db, closeDbFunc := SetupMockDBPoll(t)
	defer closeDbFunc()

	assert.Equal(t, types.Poll{Id: PollId}, db.NewPoll())
}

func TestUpdatePollError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := types.Poll{}
	forcedError := fmt.Errorf("forced poll error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdatePoll)).
		WithArgs(poll.LastPolled, poll.EnvBaseTime, poll.LastPollCompleted, poll.Id).
		WillReturnError(forcedError)

	err := db.UpdatePoll(&poll)
	assert.EqualError(t, err, forcedError.Error())
}

func TestUpdatePollRowsAffectedError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := types.Poll{}
	forcedError := fmt.Errorf("forced poll error")
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdatePoll)).
		WithArgs(poll.LastPolled, poll.EnvBaseTime, poll.LastPollCompleted, poll.Id).
		WillReturnResult(sqlmock.NewErrorResult(forcedError))

	err := db.UpdatePoll(&poll)
	assert.EqualError(t, err, forcedError.Error())
}

func TestUpdatePollInvalidId(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := types.Poll{}
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdatePoll)).
		WithArgs(poll.LastPolled, poll.EnvBaseTime, poll.LastPollCompleted, poll.Id).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := db.UpdatePoll(&poll)
	assert.True(t, strings.HasPrefix(err.Error(), "update poll updated wrong number of rows: 0, poll "))
}

func TestUpdatePoll(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDBPoll(t)
	defer closeDbFunc()

	now := time.Now()
	poll := types.Poll{
		Id:                "-1",
		LastPolled:        now,
		EnvBaseTime:       now.Add(time.Second * 1),
		LastPollCompleted: now.Add(time.Second * 2),
	}
	mock.ExpectExec(convertSqlToDbMockExpect(sqlUpdatePoll)).
		WithArgs(poll.LastPolled, poll.EnvBaseTime, poll.LastPollCompleted, poll.Id).
		WillReturnResult(sqlmock.NewResult(0, 1))

	assert.NoError(t, db.UpdatePoll(&poll))
}

func TestSelectPollError(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := types.Poll{
		Id: "-1",
	}
	forcedError := fmt.Errorf("forced select poll error")
	SetupMockPollSelectForcedError(mock, forcedError, poll.Id)

	assert.EqualError(t, db.SelectPoll(&poll), forcedError.Error())
}

func TestSelectPollInvalidId(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDBPoll(t)
	defer closeDbFunc()

	poll := types.Poll{
		Id: "-1",
	}
	mock.ExpectQuery(convertSqlToDbMockExpect(sqlSelectPoll)).
		WithArgs(poll.Id).
		WillReturnRows(sqlmock.NewRows([]string{"lastpoll", "basetime", "pollcompleted"}))

	assert.EqualError(t, db.SelectPoll(&poll), "sql: no rows in result set")
}

func TestSelectPoll(t *testing.T) {
	mock, db, closeDbFunc := SetupMockDBPoll(t)
	defer closeDbFunc()

	now := time.Now()
	poll := types.Poll{
		Id: "-1",
	}
	SetupMockPollSelect(mock, poll.Id, now)

	assert.NoError(t, db.SelectPoll(&poll))
	assert.Equal(t, types.Poll{
		Id:                "-1",
		LastPolled:        now,
		EnvBaseTime:       now.Add(time.Second * 1),
		LastPollCompleted: now.Add(time.Second * 2),
	}, poll)
}
