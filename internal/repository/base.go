package repository

import (
	"fmt"

	dbsqlc "github.com/arham/ai-second-brain/internal/db/sqlc"
)

type DBTX = dbsqlc.DBTX

type base struct {
	db      DBTX
	queries *dbsqlc.Queries
}

func newBase(db DBTX) (*base, error) {
	if db == nil {
		return nil, fmt.Errorf("repository database is required")
	}
	return &base{db: db, queries: dbsqlc.New(db)}, nil
}
