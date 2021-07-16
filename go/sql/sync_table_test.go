// Copyright 2021 The Board of Trustees of the Leland Stanford Junior University
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package sql

import (
	"database/sql"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type AnyInt64 struct{}

func (a AnyInt64) Match(v driver.Value) bool {
	_, ok := v.(int64)
	return ok
}

type SyncTableSuite struct {
	suite.Suite
	DB        *gorm.DB
	mock      sqlmock.Sqlmock
	syncTable *SyncTable
	row1      *UserDevice
	row2      *UserDevice
	record1   *UserDeviceSyncRecord
	record2   *UserDeviceSyncRecord
	record3   *UserDeviceSyncRecord
}

func (s *SyncTableSuite) SetupSuite() {
	var (
		db  *sql.DB
		err error
	)

	db, s.mock, err = sqlmock.New()
	require.NoError(s.T(), err)

	s.DB, err = gorm.Open(mysql.New(
		mysql.Config{Conn: db, SkipInitializeWithVersion: true}),
		gormConfig())

	require.NoError(s.T(), err)
	s.syncTable = NewSyncTable(s.DB)
	s.row1 = &UserDevice{
		Key:   Key{UniqueID: "u1", UserID: 1},
		State: "state1",
	}
	s.row2 = &UserDevice{
		Key:   Key{UniqueID: "u2", UserID: 1},
		State: "state2",
	}
	s.record1 = &UserDeviceSyncRecord{
		State: &s.row1.State,
		UserDeviceJournal: UserDeviceJournal{
			Key:          s.row1.Key,
			LastModified: 101,
		},
	}
	s.record2 = &UserDeviceSyncRecord{
		State: &s.row2.State,
		UserDeviceJournal: UserDeviceJournal{
			Key:          s.row2.Key,
			LastModified: 102,
		},
	}
	s.record3 = &UserDeviceSyncRecord{
		UserDeviceJournal: UserDeviceJournal{
			Key: Key{
				UniqueID: "u3",
				UserID:   1,
			},
			LastModified: 103,
		},
	}
}

func (s *SyncTableSuite) AfterTest(_, _ string) {
	require.NoError(s.T(), s.mock.ExpectationsWereMet())
}

func TestSyncTable(t *testing.T) {
	suite.Run(t, new(SyncTableSuite))
}

func (s *SyncTableSuite) TestSyncTableGetAll() {
	rows := []*UserDevice{}
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `user_device`.`uniqueId`,`user_device`.`userId`,`user_device`.`state` " +
			"FROM `user_device` WHERE userId = ?")).
		WithArgs(s.row1.Key.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"uniqueId", "userId", "state"}).
			AddRow("u1", "1", "state1").AddRow("u2", "1", "state2"))
	err := s.syncTable.GetAll(&rows, s.row1.UserID)
	require.NoError(s.T(), err)
	require.Nil(s.T(), deep.Equal([]*UserDevice{s.row1, s.row2}, rows))
}

func (s *SyncTableSuite) TestSyncTableGetOne() {
	row := &UserDevice{Key: s.row1.Key}
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `user_device`.`uniqueId`,`user_device`.`userId`,`user_device`.`state` FROM `user_device` " +
			"WHERE `user_device`.`uniqueId` = ? AND `user_device`.`userId` = ? " +
			"ORDER BY `user_device`.`uniqueId` LIMIT 1")).
		WillReturnRows(sqlmock.NewRows([]string{"uniqueId", "userId", "state"}).
			AddRow("u1", "1", "state1"))
	err := s.syncTable.GetOne(row)
	require.NoError(s.T(), err)
	require.Nil(s.T(), deep.Equal(s.row1, row))
}

func (s *SyncTableSuite) TestSyncTableGetRaw() {
	row := &UserDevice{}
	r1 := s.record1
	r2 := s.record2
	key1 := r1.UserDeviceJournal.Key
	key2 := r2.UserDeviceJournal.Key
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"select tj.uniqueId,tj.userId,tj.lastModified,t.state from user_device_journal as tj " +
			"left outer join user_device as t on tj.uniqueId = t.uniqueId and tj.userId = t.userId " +
			"where tj.userId = ?")).
		WithArgs(s.row1.Key.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"uniqueId", "userId", "lastModified", "state"}).
			AddRow(key1.UniqueID, key1.UserID, r1.GetLastModified(), r1.State).
			AddRow(key2.UniqueID, key2.UserID, r2.GetLastModified(), r2.State))
	rows, err := s.syncTable.GetRaw(row, s.row1.UserID)
	require.NoError(s.T(), err)
	want, err := ToSyncRecordSlice(&[]*UserDeviceSyncRecord{r1, r2})
	require.NoError(s.T(), err)
	require.Nil(s.T(), deep.Equal(want, rows))
}

func (s *SyncTableSuite) TestSyncTableGetChangesAfter() {
	row := &UserDevice{}
	want := s.record2
	key := s.record2.UserDeviceJournal.Key
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"select tj.uniqueId,tj.userId,tj.lastModified,t.state from user_device_journal as tj "+
			"left outer join user_device as t on tj.uniqueId = t.uniqueId and tj.userId = t.userId "+
			"where tj.lastModified > ? and tj.userId = ?")).
		WithArgs(100, s.row1.Key.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"uniqueId", "userId", "lastModified", "state"}).
			AddRow(key.UniqueID, key.UserID, want.GetLastModified(), want.State))
	rows, err := s.syncTable.GetChangesAfter(row, 100, s.row1.UserID)
	require.NoError(s.T(), err)
	wantRows, err := ToSyncRecordSlice(&[]*UserDeviceSyncRecord{want})
	require.NoError(s.T(), err)
	require.Nil(s.T(), deep.Equal(wantRows, rows))
}

func (s *SyncTableSuite) TestSyncTableHandleChanges() {
	changes, err := ToSyncRecordSlice(&[]*UserDeviceSyncRecord{
		s.record1, // not recent
		s.record2, // insert
		s.record3, // delete
	})
	require.NoError(s.T(), err)

	s.mock.ExpectBegin()
	// record1: not recent
	journal := s.record1.UserDeviceJournal
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `user_device_journal`.`lastModified` FROM `user_device_journal` "+
			"WHERE uniqueId = ? AND userId = ? "+
			"ORDER BY `user_device_journal`.`uniqueId` LIMIT 1")).
		WithArgs(journal.Key.UniqueID, journal.Key.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"lastModified"}).
			AddRow(101))
	// record2: insert
	journal = s.record2.UserDeviceJournal
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `user_device_journal`.`lastModified` FROM `user_device_journal` "+
			"WHERE uniqueId = ? AND userId = ? "+
			"ORDER BY `user_device_journal`.`uniqueId` LIMIT 1")).
		WithArgs(journal.Key.UniqueID, journal.Key.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"lastModified"}).
			AddRow(101))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `user_device` (`uniqueId`,`userId`,`state`) VALUES (?,?,?) "+
			"ON DUPLICATE KEY UPDATE `state`=VALUES(`state`)")).
		WithArgs(journal.Key.UniqueID, journal.Key.UserID, s.record2.State).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `user_device_journal` (`uniqueId`,`userId`,`lastModified`) VALUES (?,?,?) "+
			"ON DUPLICATE KEY UPDATE `lastModified`=VALUES(`lastModified`)")).
		WithArgs(journal.Key.UniqueID, journal.Key.UserID, journal.LastModified).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// record3: delete
	journal = s.record3.UserDeviceJournal
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `user_device_journal`.`lastModified` FROM `user_device_journal` "+
			"WHERE uniqueId = ? AND userId = ? "+
			"ORDER BY `user_device_journal`.`uniqueId` LIMIT 1")).
		WithArgs(journal.Key.UniqueID, journal.Key.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"lastModified"}).
			AddRow(101))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `user_device` "+
			"WHERE (`user_device`.`uniqueId`,`user_device`.`userId`) IN ((?,?))")).
		WithArgs(journal.Key.UniqueID, journal.Key.UserID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `user_device_journal` (`uniqueId`,`userId`,`lastModified`) VALUES (?,?,?) "+
			"ON DUPLICATE KEY UPDATE `lastModified`=VALUES(`lastModified`)")).
		WithArgs(journal.Key.UniqueID, journal.Key.UserID, journal.LastModified).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectCommit()
	results, err := s.syncTable.HandleChanges(changes, journal.Key.UserID)
	require.NoError(s.T(), err)
	require.Nil(s.T(), deep.Equal([]bool{false, true, true}, results))
}

func (s *SyncTableSuite) TestSyncTableSyncAt() {
	syncChange := s.record1
	ourChange := s.record2
	pushedChanges, err := ToSyncRecordSlice(&[]*UserDeviceSyncRecord{
		s.record3, // delete
	})
	require.NoError(s.T(), err)

	s.mock.ExpectBegin()
	// getChangesAfter
	journal := ourChange.UserDeviceJournal
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"select tj.uniqueId,tj.userId,tj.lastModified,t.state from user_device_journal as tj "+
			"left outer join user_device as t on tj.uniqueId = t.uniqueId and tj.userId = t.userId "+
			"where tj.lastModified > ? and tj.userId = ?")).
		WithArgs(syncChange.UserDeviceJournal.LastModified, syncChange.UserDeviceJournal.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"uniqueId", "userId", "lastModified", "state"}).
			AddRow(journal.UniqueID, journal.UserID, journal.LastModified, ourChange.State))
	// getLastModified
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"select max(lastModified) as max_last_modified from user_device_journal where userId = ?")).
		WithArgs(syncChange.UserDeviceJournal.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"max_last_modified"}).
			AddRow(ourChange.GetLastModified()))
	// handleChanges record3: delete
	journal = s.record3.UserDeviceJournal
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `user_device_journal`.`lastModified` FROM `user_device_journal` "+
			"WHERE uniqueId = ? AND userId = ? "+
			"ORDER BY `user_device_journal`.`uniqueId` LIMIT 1")).
		WithArgs(journal.Key.UniqueID, journal.Key.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"lastModified"}).
			AddRow(101))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `user_device` "+
			"WHERE (`user_device`.`uniqueId`,`user_device`.`userId`) IN ((?,?))")).
		WithArgs(journal.Key.UniqueID, journal.Key.UserID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `user_device_journal` (`uniqueId`,`userId`,`lastModified`) VALUES (?,?,?) "+
			"ON DUPLICATE KEY UPDATE `lastModified`=VALUES(`lastModified`)")).
		WithArgs(journal.Key.UniqueID, journal.Key.UserID, journal.LastModified).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectCommit()

	lastModified, gotOurChanges, results, err := s.syncTable.SyncAt(syncChange, pushedChanges)
	require.NoError(s.T(), err)
	require.Equal(s.T(), ourChange.GetLastModified(), lastModified)
	require.Nil(s.T(), deep.Equal([]SyncRecord{ourChange}, gotOurChanges))
	require.Nil(s.T(), deep.Equal([]bool{true}, results))
}

func (s *SyncTableSuite) TestSyncTableReplaceAll() {
	changes, err := ToSyncRecordSlice(&[]*UserDeviceSyncRecord{
		s.record1, // insert
		s.record2, // insert
		s.record3, // skip
	})
	require.NoError(s.T(), err)

	s.mock.ExpectBegin()
	journal := s.record1.UserDeviceJournal
	s.mock.ExpectExec(regexp.QuoteMeta(
		"delete from user_device where userId = ?")).
		WithArgs(journal.Key.UserID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"delete from user_device_journal where userId = ?")).
		WithArgs(journal.Key.UserID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// record1: insert
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `user_device` (`uniqueId`,`userId`,`state`) VALUES (?,?,?) "+
			"ON DUPLICATE KEY UPDATE `state`=VALUES(`state`)")).
		WithArgs(journal.Key.UniqueID, journal.Key.UserID, s.record1.State).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `user_device_journal` (`uniqueId`,`userId`,`lastModified`) VALUES (?,?,?) "+
			"ON DUPLICATE KEY UPDATE `lastModified`=VALUES(`lastModified`)")).
		WithArgs(journal.Key.UniqueID, journal.Key.UserID, journal.LastModified).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// record2: insert
	journal = s.record2.UserDeviceJournal
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `user_device` (`uniqueId`,`userId`,`state`) VALUES (?,?,?) "+
			"ON DUPLICATE KEY UPDATE `state`=VALUES(`state`)")).
		WithArgs(journal.Key.UniqueID, journal.Key.UserID, s.record2.State).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `user_device_journal` (`uniqueId`,`userId`,`lastModified`) VALUES (?,?,?) "+
			"ON DUPLICATE KEY UPDATE `lastModified`=VALUES(`lastModified`)")).
		WithArgs(journal.Key.UniqueID, journal.Key.UserID, journal.LastModified).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// record3: skip
	s.mock.ExpectCommit()
	err = s.syncTable.ReplaceAll(changes, journal.Key.UserID)
	require.NoError(s.T(), err)
}

func (s *SyncTableSuite) TestSyncTableInsertIfRecent() {
	row := s.row1
	// Test if recent
	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `user_device_journal`.`lastModified` FROM `user_device_journal` "+
			"WHERE uniqueId = ? AND userId = ? "+
			"ORDER BY `user_device_journal`.`uniqueId` LIMIT 1")).
		WithArgs(row.Key.UniqueID, row.Key.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"lastModified"}).
			AddRow(101))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `user_device` (`uniqueId`,`userId`,`state`) VALUES (?,?,?) "+
			"ON DUPLICATE KEY UPDATE `state`=VALUES(`state`)")).
		WithArgs(row.Key.UniqueID, row.Key.UserID, row.State).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `user_device_journal` (`uniqueId`,`userId`,`lastModified`) VALUES (?,?,?) "+
			"ON DUPLICATE KEY UPDATE `lastModified`=VALUES(`lastModified`)")).
		WithArgs(row.Key.UniqueID, row.Key.UserID, 200).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectCommit()
	done, err := s.syncTable.InsertIfRecent(row, 200)
	require.NoError(s.T(), err)
	require.Equal(s.T(), done, true)
	// Test if not recent
	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `user_device_journal`.`lastModified` FROM `user_device_journal` "+
			"WHERE uniqueId = ? AND userId = ? "+
			"ORDER BY `user_device_journal`.`uniqueId` LIMIT 1")).
		WithArgs(row.Key.UniqueID, row.Key.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"lastModified"}).
			AddRow("101"))
	s.mock.ExpectCommit()
	done, err = s.syncTable.InsertIfRecent(row, 100)
	require.NoError(s.T(), err)
	require.Equal(s.T(), done, false)
}

func (s *SyncTableSuite) TestSyncTableInsertOne() {
	row := s.row1
	now := time.Now().UnixNano() / 1e6
	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `user_device` (`uniqueId`,`userId`,`state`) VALUES (?,?,?) "+
			"ON DUPLICATE KEY UPDATE `state`=VALUES(`state`)")).
		WithArgs(row.Key.UniqueID, row.Key.UserID, row.State).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `user_device_journal` (`uniqueId`,`userId`,`lastModified`) VALUES (?,?,?) "+
			"ON DUPLICATE KEY UPDATE `lastModified`=VALUES(`lastModified`)")).
		WithArgs(row.Key.UniqueID, row.Key.UserID, AnyInt64{}).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectCommit()
	lastModified, err := s.syncTable.InsertOne(row)
	require.NoError(s.T(), err)
	require.GreaterOrEqual(s.T(), lastModified, now)
}

func (s *SyncTableSuite) TestSyncTableDeleteIfRecent() {
	row := &UserDevice{Key: s.row1.Key}
	// if recent
	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `user_device_journal`.`lastModified` FROM `user_device_journal` "+
			"WHERE uniqueId = ? AND userId = ? "+
			"ORDER BY `user_device_journal`.`uniqueId` LIMIT 1")).
		WithArgs(row.Key.UniqueID, row.Key.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"lastModified"}).
			AddRow(101))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `user_device` "+
			"WHERE (`user_device`.`uniqueId`,`user_device`.`userId`) IN ((?,?))")).
		WithArgs(row.Key.UniqueID, row.Key.UserID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `user_device_journal` (`uniqueId`,`userId`,`lastModified`) VALUES (?,?,?) "+
			"ON DUPLICATE KEY UPDATE `lastModified`=VALUES(`lastModified`)")).
		WithArgs(row.Key.UniqueID, row.Key.UserID, AnyInt64{}).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectCommit()
	done, err := s.syncTable.DeleteIfRecent(row, 200)
	require.NoError(s.T(), err)
	require.Equal(s.T(), done, true)
	// if not recent
	s.mock.ExpectBegin()
	s.mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `user_device_journal`.`lastModified` FROM `user_device_journal` "+
			"WHERE uniqueId = ? AND userId = ? "+
			"ORDER BY `user_device_journal`.`uniqueId` LIMIT 1")).
		WithArgs(row.Key.UniqueID, row.Key.UserID).
		WillReturnRows(sqlmock.NewRows([]string{"lastModified"}).
			AddRow(101))
	s.mock.ExpectCommit()
	done, err = s.syncTable.DeleteIfRecent(row, 100)
	require.NoError(s.T(), err)
	require.Equal(s.T(), done, false)
}

func (s *SyncTableSuite) TestSyncTableDeleteOne() {
	row := &UserDevice{Key: s.row1.Key}
	now := time.Now().UnixNano() / 1e6
	s.mock.ExpectBegin()
	s.mock.ExpectExec(regexp.QuoteMeta(
		"DELETE FROM `user_device` "+
			"WHERE (`user_device`.`uniqueId`,`user_device`.`userId`) IN ((?,?))")).
		WithArgs(row.Key.UniqueID, row.Key.UserID).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO `user_device_journal` (`uniqueId`,`userId`,`lastModified`) VALUES (?,?,?) "+
			"ON DUPLICATE KEY UPDATE `lastModified`=VALUES(`lastModified`)")).
		WithArgs(row.Key.UniqueID, row.Key.UserID, AnyInt64{}).
		WillReturnResult(sqlmock.NewResult(1, 1))
	s.mock.ExpectCommit()
	lastModified, err := s.syncTable.DeleteOne(row)
	require.NoError(s.T(), err)
	require.GreaterOrEqual(s.T(), lastModified, now)
}
