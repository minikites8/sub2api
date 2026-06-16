package service

import (
	dbent "github.com/Wei-Shaw/sub2api/ent"

	"entgo.io/ent/dialect"
)

func paymentTxSupportsForUpdate(tx *dbent.Tx) bool {
	if tx == nil || tx.Client() == nil {
		return false
	}
	driver := tx.Client().Driver()
	return driver != nil && driver.Dialect() == dialect.Postgres
}
