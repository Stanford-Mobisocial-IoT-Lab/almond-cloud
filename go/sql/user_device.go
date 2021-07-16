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

// UserDevice table
type UserDevice struct {
	Key
	State string `json:"state" gorm:"column:state"`
}

// TableName overrides table name to `user_device`
func (*UserDevice) TableName() string {
	return "user_device"
}

// NewRow returns a UserDevice row
func (*UserDevice) NewRow() Row {
	return &UserDevice{}
}

// NewRows returns a slice of UserDevice row
func (*UserDevice) NewRows() interface{} {
	return &[]*UserDevice{}
}

// SetKey sets the key of UserDevice
func (e *UserDevice) SetKey(key Key) {
	e.Key = key
}

// NewSyncRecord
func (e *UserDevice) NewSyncRecord(lastModified int64) SyncRecord {
	return &UserDeviceSyncRecord{
		UserDeviceJournal: UserDeviceJournal{
			Key:          e.Key,
			LastModified: lastModified,
		},
		State: &e.State,
	}
}

// NewSyncRecords
func (e *UserDevice) NewSyncRecords() interface{} {
	return &[]*UserDeviceSyncRecord{}
}

// GetKey returns the key of UserDevice
func (e *UserDevice) GetKey() Key {
	return e.Key
}

// Fields returns the column names without Key
func (e *UserDevice) Fields() []string {
	return []string{"state"}
}

// UserDeviceJournal table
type UserDeviceJournal struct {
	Key
	LastModified int64 `json:"lastModified" gorm:"column:lastModified"`
}

// TableName overrides table name to `user_device`
func (*UserDeviceJournal) TableName() string {
	return "user_device_journal"
}

// NewRow returns a UserDevice row
func (*UserDeviceJournal) NewRow() Row {
	return &UserDeviceJournal{}
}

// NewRows returns a slice of UserDevice row
func (*UserDeviceJournal) NewRows() interface{} {
	return &[]UserDeviceJournal{}
}

// SetKey sets the key of UserDevice
func (e *UserDeviceJournal) SetKey(key Key) {
	e.Key = key
}

// GetKey returns the key of UserDevice
func (e *UserDeviceJournal) GetKey() Key {
	return e.Key
}

// Fields returns the column names without Key
func (e *UserDeviceJournal) Fields() []string {
	return []string{"lastModified"}
}

// UserDeviceSyncRecord is the joined row of UserDevice and UserDeviceJournal
type UserDeviceSyncRecord struct {
	UserDeviceJournal
	State *string `json:"state" gorm:"column:state"`
}

// Row
func (r *UserDeviceSyncRecord) Row() SyncRow {
	return &UserDevice{
		Key:   r.UserDeviceJournal.Key,
		State: *r.State,
	}
}

// JournalRow
func (r *UserDeviceSyncRecord) JournalRow() Row {
	return &r.UserDeviceJournal
}

// GetLastModified
func (r *UserDeviceSyncRecord) GetLastModified() int64 {
	return r.UserDeviceJournal.LastModified
}

// SetLastModified
func (r *UserDeviceSyncRecord) SetLastModified(t int64) {
	r.UserDeviceJournal.LastModified = t
}

// HasDiscriminator
func (r *UserDeviceSyncRecord) HasDiscriminator() bool {
	return r.State != nil && len(*r.State) > 0
}
