package model

import (
	"database/sql"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// LogExportQuery captures the filter set shared by the admin and self
// CSV export endpoints. UserID is the only column that can be set in
// the self view; the admin view leaves it zero and uses Username to
// look up matching rows.
type LogExportQuery struct {
	LogType           int
	StartTimestamp    int64
	EndTimestamp      int64
	Username          string
	TokenName         string
	ModelName         string
	Channel           int
	Group             string
	RequestId         string
	UpstreamRequestId string
	UserID            int
	Limit             int
}

// LogExportRowIterator streams log rows one at a time. The Scan method
// must be called after Next reports true; Close releases the underlying
// database rows.
type LogExportRowIterator struct {
	rows *sql.Rows
}

func (it *LogExportRowIterator) Next() bool {
	if it.rows == nil {
		return false
	}
	return it.rows.Next()
}

func (it *LogExportRowIterator) Scan() (*Log, error) {
	if it.rows == nil {
		return nil, errors.New("iterator closed")
	}
	log := &Log{}
	if err := it.rows.Scan(log); err != nil {
		return nil, err
	}
	return log, nil
}

func (it *LogExportRowIterator) Err() error {
	if it.rows == nil {
		return nil
	}
	return it.rows.Err()
}

func (it *LogExportRowIterator) Close() {
	if it.rows == nil {
		return
	}
	if err := it.rows.Close(); err != nil {
		common.SysError("failed to close log export iterator: " + err.Error())
	}
	it.rows = nil
}

// StreamLogsForExport builds a database query matching the supplied
// filters and returns a streaming iterator. The caller is responsible
// for calling Close to release the connection. Limit <= 0 disables the
// row cap, which is suitable only for the admin endpoint.
func StreamLogsForExport(query LogExportQuery) (*LogExportRowIterator, error) {
	tx, err := buildLogExportQuery(query)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Rows()
	if err != nil {
		return nil, err
	}
	return &LogExportRowIterator{rows: rows}, nil
}

func buildLogExportQuery(query LogExportQuery) (*gorm.DB, error) {
	tx := LOG_DB.Model(&Log{})
	if query.LogType != LogTypeUnknown {
		tx = tx.Where("logs.type = ?", query.LogType)
	}
	if query.UserID > 0 {
		tx = tx.Where("logs.user_id = ?", query.UserID)
	}
	var err error
	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", query.ModelName); err != nil {
		return nil, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.username", query.Username); err != nil {
		return nil, err
	}
	if query.TokenName != "" {
		tx = tx.Where("logs.token_name = ?", query.TokenName)
	}
	if query.RequestId != "" {
		tx = tx.Where("logs.request_id = ?", query.RequestId)
	}
	if query.UpstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", query.UpstreamRequestId)
	}
	if query.StartTimestamp > 0 {
		tx = tx.Where("logs.created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp > 0 {
		tx = tx.Where("logs.created_at <= ?", query.EndTimestamp)
	}
	if query.Channel > 0 {
		tx = tx.Where("logs.channel_id = ?", query.Channel)
	}
	if query.Group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", query.Group)
	}

	order := "logs.created_at desc, logs.id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("logs.")
	}
	tx = tx.Order(order)
	if query.Limit > 0 {
		tx = tx.Limit(query.Limit)
	}
	return tx, nil
}

