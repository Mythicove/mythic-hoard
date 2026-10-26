// Package migrate 在启动时执行 goose 迁移。
package migrate

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/Mythicove/mythic-hoard/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// advisoryLockKey 是迁移串行化用的 PG advisory lock 键（与 mirror 相同：混合部署共库时
// 顺带串行化两服务的 DDL；各自版本表独立，锁只防并发竞态）。
const advisoryLockKey int64 = 0x6d797468 // "myth"

// Run 对 dsn 指向的数据库执行迁移（advisory-lock 串行化，多副本安全）。
func Run(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// pg_advisory_lock 会阻塞直到取得锁；用独立连接持有到迁移结束。
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", advisoryLockKey); err != nil {
		return err
	}
	//nolint:errcheck // 连接关闭时锁自动释放；显式解锁仅为尽早释放。
	defer conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", advisoryLockKey)

	// goose 的版本表建在 search_path 的首个可用 schema 中，而该 schema 本身由迁移创建 —— 鸡生蛋：
	// DSN 带 options=-csearch_path=hoard（与 mirror 共库时的 schema 隔离）而 schema 尚不存在时，
	// goose 建表会失败（ERROR: no schema has been selected to create in, SQLSTATE 3F000）。
	if err := ensureSearchPathSchemas(ctx, conn); err != nil {
		return err
	}

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	// 与 Mirror 共享同一 Postgres：使用独立版本表避免冲突。
	goose.SetTableName("hoard_goose_version")
	return goose.Up(db, ".")
}

// ensureSearchPathSchemas 为 search_path 中尚不存在的 schema 执行 CREATE SCHEMA IF NOT EXISTS。
// schema 名取自连接的实际 search_path（不硬编码），跳过 "$user" 这类占位项；已存在的为 no-op。
func ensureSearchPathSchemas(ctx context.Context, conn *sql.Conn) error {
	var searchPath string
	if err := conn.QueryRowContext(ctx, "SHOW search_path").Scan(&searchPath); err != nil {
		return err
	}
	for _, raw := range strings.Split(searchPath, ",") {
		name := strings.TrimSpace(strings.Trim(raw, `"`))
		if name == "" || strings.Contains(name, "$") {
			continue
		}
		stmt := `CREATE SCHEMA IF NOT EXISTS "` + strings.ReplaceAll(name, `"`, `""`) + `"`
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
