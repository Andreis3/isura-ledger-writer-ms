//go:build unit

package repository

import (
	"context"
	"reflect"

	"github.com/andreis3/isura-ledger-ms/internal/infra/postgres/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ database.Querier = (*QuerierFake)(nil)
var _ pgx.Row = RowFake{}
var _ pgx.Rows = (*RowsFake)(nil)
var _ pgx.BatchResults = (*BatchResultsFake)(nil)

// QuerierFake provides deterministic database responses for adapter unit tests.
type QuerierFake struct {
	ExecErr   error
	QueryErr  error
	Rows      pgx.Rows
	QueryRows []pgx.Rows
	QueryIdx  int
	Row       pgx.Row
	Batch     pgx.BatchResults
	ExecCalls int
	Queries   []string
	Arguments [][]any
}

func (f *QuerierFake) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	f.Queries = append(f.Queries, query)
	f.Arguments = append(f.Arguments, args)
	return pgconn.NewCommandTag("INSERT 0 1"), f.ExecErr
}

func (f *QuerierFake) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	f.Queries = append(f.Queries, query)
	f.Arguments = append(f.Arguments, args)
	if f.QueryErr != nil {
		return nil, f.QueryErr
	}
	if f.QueryIdx < len(f.QueryRows) {
		rows := f.QueryRows[f.QueryIdx]
		f.QueryIdx++
		return rows, nil
	}
	if f.Rows == nil {
		return &RowsFake{}, nil
	}
	return f.Rows, nil
}

func (f *QuerierFake) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	f.Queries = append(f.Queries, query)
	f.Arguments = append(f.Arguments, args)
	if f.Row == nil {
		return RowFake{Err: f.QueryErr}
	}
	return f.Row
}

func (f *QuerierFake) SendBatch(_ context.Context, batch *pgx.Batch) pgx.BatchResults {
	f.ExecCalls = len(batch.QueuedQueries)
	if f.Batch != nil {
		return f.Batch
	}
	return &BatchResultsFake{}
}

// RowFake scans configured values into destinations or returns Err.
type RowFake struct {
	Values []any
	Err    error
}

func (r RowFake) Scan(dest ...any) error {
	if r.Err != nil {
		return r.Err
	}
	if len(dest) != len(r.Values) {
		return pgx.ErrNoRows
	}
	for index, value := range r.Values {
		target := reflect.ValueOf(dest[index])
		if target.Kind() != reflect.Pointer || !target.Elem().CanSet() {
			return pgx.ErrNoRows
		}
		valueRef := reflect.ValueOf(value)
		if !valueRef.IsValid() || !valueRef.Type().AssignableTo(target.Elem().Type()) {
			return pgx.ErrNoRows
		}
		target.Elem().Set(valueRef)
	}
	return nil
}

// RowsFake is an empty result set unless a query error is configured.
type RowsFake struct {
	ScanErr error
	RowsErr error
	Closed  bool
	Data    [][]any
	index   int
}

func (r *RowsFake) Close()                                     { r.Closed = true }
func (r *RowsFake) Err() error                                 { return r.RowsErr }
func (*RowsFake) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (*RowsFake) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *RowsFake) Next() bool {
	if r.index >= len(r.Data) {
		return false
	}
	r.index++
	return true
}
func (r *RowsFake) Scan(dest ...any) error {
	if r.ScanErr != nil {
		return r.ScanErr
	}
	if r.index == 0 || r.index > len(r.Data) || len(dest) != len(r.Data[r.index-1]) {
		return pgx.ErrNoRows
	}
	for column, value := range r.Data[r.index-1] {
		target := reflect.ValueOf(dest[column])
		valueRef := reflect.ValueOf(value)
		if target.Kind() != reflect.Pointer || !target.Elem().CanSet() || !valueRef.IsValid() || !valueRef.Type().AssignableTo(target.Elem().Type()) {
			return pgx.ErrNoRows
		}
		target.Elem().Set(valueRef)
	}
	return nil
}
func (*RowsFake) Values() ([]any, error) { return nil, nil }
func (*RowsFake) RawValues() [][]byte    { return nil }
func (*RowsFake) Conn() *pgx.Conn        { return nil }

// BatchResultsFake returns configured errors in command order.
type BatchResultsFake struct {
	Errors  []error
	ExecIdx int
	Closed  bool
}

func (r *BatchResultsFake) Exec() (pgconn.CommandTag, error) {
	index := r.ExecIdx
	r.ExecIdx++
	if index < len(r.Errors) {
		return pgconn.CommandTag{}, r.Errors[index]
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}
func (*BatchResultsFake) Query() (pgx.Rows, error) { return &RowsFake{}, nil }
func (*BatchResultsFake) QueryRow() pgx.Row        { return RowFake{} }
func (r *BatchResultsFake) Close() error           { r.Closed = true; return nil }
