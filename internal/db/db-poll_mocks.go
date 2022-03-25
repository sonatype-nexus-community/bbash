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
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
	"testing"
	"time"
)

// SetupMockDBPoll should always be followed by a call to the closeDbFunc, like so:
// 	mock, db, closeDbFunc := SetupMockDBPoll(t)
//	defer closeDbFunc()
func SetupMockDBPoll(t *testing.T) (mock sqlmock.Sqlmock, mockDbPoll *PollStruct, closeDbFunc func()) {
	db, mock, err := sqlmock.New()
	if err != nil {
		assert.NoError(t, err)
	}
	closeDbFunc = func() {
		_ = db.Close()
	}
	mockDbPoll = NewDBPoll(db, zaptest.NewLogger(t))
	return
}

// PollConvertSqlToDbMockExpect exposed for poll package tests
func PollConvertSqlToDbMockExpect(realSql string) string {
	return convertSqlToDbMockExpect(realSql)
}

func SetupMockPollSelectForcedError(mock sqlmock.Sqlmock, forcedError error, pollId string) {
	mock.ExpectQuery(PollConvertSqlToDbMockExpect(sqlSelectPoll)).
		WithArgs(pollId).
		WillReturnError(forcedError)
}

func setupMockPollSelect(mock sqlmock.Sqlmock, pollId string, now time.Time) {
	mock.ExpectQuery(PollConvertSqlToDbMockExpect(sqlSelectPoll)).
		WithArgs(pollId).
		WillReturnRows(sqlmock.NewRows([]string{"lastpoll", "basetime", "pollcompleted"}).
			AddRow(now, now.Add(time.Second*1), now.Add(time.Second*2)))
}

func SetupMockPollSelectAndUpdate(mock sqlmock.Sqlmock, pollId string, now time.Time, rowsAffected int64) {
	setupMockPollSelect(mock, pollId, now)

	// expect call to UpdatePoll too
	mock.ExpectExec(PollConvertSqlToDbMockExpect(sqlUpdatePoll)).
		WithArgs(now, sqlmock.AnyArg(), sqlmock.AnyArg(), pollId).
		WillReturnResult(sqlmock.NewResult(0, rowsAffected))
}

func SetupMockPollSelectAndUpdateAnyUpdateTime(mock sqlmock.Sqlmock, pollId string, now time.Time, rowsAffected int64) {
	setupMockPollSelect(mock, pollId, now)

	// expect call to UpdatePoll too
	mock.ExpectExec(PollConvertSqlToDbMockExpect(sqlUpdatePoll)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), pollId).
		WillReturnResult(sqlmock.NewResult(0, rowsAffected))
}
