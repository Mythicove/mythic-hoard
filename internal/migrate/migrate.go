// Package migrate 在启动时执行 goose 迁移。
package migrate

import (
	"database/sql"

	"github.com/Mythicove/mythic-hoard/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Run 对 dsn 指向的数据库执行迁移。
func Run(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	// 与 Mirror 共享同一 Postgres：使用独立版本表避免冲突。
	goose.SetTableName("hoard_goose_version")
	return goose.Up(db, ".")
}
