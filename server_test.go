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
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"testing"
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

func TestMigrateDBErrorMigrateUp(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()

	setupMockPostgresWithInstance(mock)

	assert.EqualError(t, migrateDB(dbMock), "try lock failed in line 0: SELECT pg_advisory_lock($1) (details: all expectations were already fulfilled, call to ExecQuery 'SELECT pg_advisory_lock($1)' with args [{Name: Ordinal:1 Value:1014225327}] was not expected)")
}

func setupMockPostgresWithInstance(mock sqlmock.Sqlmock) (args []driver.Value) {
	// mocks for 'postgres.WithInstance()'
	mock.ExpectQuery("SELECT CURRENT_DATABASE()").
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).FromCSVString("theDatabaseName"))
	mock.ExpectQuery("SELECT CURRENT_SCHEMA()").
		WillReturnRows(sqlmock.NewRows([]string{"col1"}).FromCSVString("theDatabaseSchema"))

	args = []driver.Value{"1014225327"}
	mock.ExpectExec("SELECT pg_advisory_lock\\(\\$1\\)").
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS \"schema_migrations\" \\(version bigint not null primary key, dirty boolean not null\\)").
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 0))
	return
}

func xxxTestMigrateDB(t *testing.T) {
	dbMock, mock := newMockDb(t)
	defer func() {
		_ = dbMock.Close()
	}()

	args := setupMockPostgresWithInstance(mock)

	// mocks for the migrate.Up()
	mock.ExpectExec("SELECT pg_advisory_lock\\(\\$1\\)").
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery("SELECT version, dirty FROM \"schema_migrations\" LIMIT 1").
		WillReturnRows(sqlmock.NewRows([]string{"version", "dirty"}).FromCSVString("-1,false"))

	mock.ExpectBegin()
	mock.ExpectExec("TRUNCATE \"schema_migrations\"").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO \"schema_migrations\" \\(version, dirty\\) VALUES \\(\\$1, \\$2\\)").
		WithArgs(1, true).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	mock.ExpectExec("BEGIN; CREATE EXTENSION pgcrypto; CREATE TABLE teams*").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectBegin()
	mock.ExpectExec("TRUNCATE \"schema_migrations\"").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO \"schema_migrations\" \\(version, dirty\\) VALUES \\(\\$1, \\$2\\)").
		WithArgs(1, false).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 0))

	assert.NoError(t, migrateDB(dbMock))
}
