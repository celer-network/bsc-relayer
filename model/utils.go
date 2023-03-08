package model

import (
	"context"
	_ "embed"

	"github.com/celer-network/goutils/log"
)

//go:embed schema.sql
var schema string

func (q *Queries) ApplySchema() error {
	if q.IsNil() {
		return nil
	}
	_, err := q.db.ExecContext(context.Background(), schema)
	if err != nil {
		return err
	}
	log.Infoln("schema sync'd")
	return nil
}

func (q *Queries) IsNil() bool {
	if q == nil || q.db == nil {
		return true
	}
	return false
}
