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
	"fmt"
	"github.com/sonatype-nexus-community/bbash/internal/types"
	"go.uber.org/zap"
)

type IDBPoll interface {
	GetLogger() *zap.Logger
	NewPoll() types.Poll
	UpdatePoll(poll *types.Poll) (err error)
	SelectPoll(poll *types.Poll) (err error)
}

type PollStruct struct {
	db     *sql.DB
	logger *zap.Logger
}

func (p *PollStruct) GetLogger() *zap.Logger {
	return p.logger
}

// enforce implementation of interface
var _ IDBPoll = (*PollStruct)(nil)

func NewDBPoll(db *sql.DB, logger *zap.Logger) *PollStruct {
	return &PollStruct{db: db, logger: logger}
}

// PollId there can be only one
const PollId = "1"

func NewPoll() types.Poll {
	return types.Poll{
		Id: PollId,
	}
}
func (p *PollStruct) NewPoll() types.Poll {
	return NewPoll()
}

const sqlUpdatePoll = `UPDATE poll
		SET 
			last_polled_on=$1, 
			env_base_time=$2, 
			last_poll_completed=$3
		WHERE poll_instance=$4`

func (p *PollStruct) UpdatePoll(poll *types.Poll) (err error) {
	var res sql.Result
	res, err = p.db.Exec(sqlUpdatePoll, poll.LastPolled, poll.EnvBaseTime, poll.LastPollCompleted, poll.Id)
	if err != nil {
		return
	}

	var rowsAffected int64
	rowsAffected, err = res.RowsAffected()
	if err != nil {
		return
	}
	if rowsAffected != 1 {
		err = fmt.Errorf("update poll updated wrong number of rows: %d, poll %+v", rowsAffected, poll)
	}
	return
}

const sqlSelectPoll = `SELECT 
			last_polled_on, 
			env_base_time, 
			last_poll_completed
        FROM poll
		WHERE poll_instance=$1`

func (p *PollStruct) SelectPoll(poll *types.Poll) (err error) {
	row := p.db.QueryRow(sqlSelectPoll, poll.Id)

	err = row.Scan(
		&poll.LastPolled,
		&poll.EnvBaseTime,
		&poll.LastPollCompleted,
	)
	if err != nil {
		p.logger.Error("selectPoll scan error", zap.Error(err))
		return
	}
	return
}
