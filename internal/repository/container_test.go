package repository

import (
	"context"
	"database/sql"
	"testing"
)

type stubDBTX struct{}

func (stubDBTX) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, nil
}

func (stubDBTX) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, nil
}

func (stubDBTX) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, nil
}

func (stubDBTX) QueryRowContext(context.Context, string, ...any) *sql.Row {
	return nil
}

func TestNewContainerRequiresDatabase(t *testing.T) {
	if _, err := NewContainer(nil); err == nil {
		t.Fatal("NewContainer(nil) error = nil, want error")
	}
}

func TestNewContainerPopulatesDependencies(t *testing.T) {
	container, err := NewContainer(stubDBTX{})
	if err != nil {
		t.Fatalf("NewContainer() error = %v", err)
	}
	if container == nil || container.Health == nil {
		t.Fatal("repository container has nil required dependency")
	}
}
