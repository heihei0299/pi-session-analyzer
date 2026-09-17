package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const SchemaVersion = 3

// Database 封装 sql.DB，对应 cc-switch schema.rs
type Database struct {
	DB   *sql.DB
	Path string
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			if p == "~" {
				return home
			}
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// ResolveDbPath 按优先级解析：envDb > dbPath > ~/.cache > data（默认不共库，显式 env/--db 才共库，空白归一）
func ResolveDbPath(dbPath, envDb string) string {
	if strings.TrimSpace(envDb) != "" {
		return expandHome(strings.TrimSpace(envDb))
	}
	if strings.TrimSpace(dbPath) != "" {
		return expandHome(strings.TrimSpace(dbPath))
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" && home != "/" {
		return filepath.Join(home, ".cache", "token-analyzer", "token-analyzer.db")
	}
	return filepath.Join("data", "token-analyzer.db")
}

func ResolveDbPathFromEnv(dbPath string) string {
	return ResolveDbPath(dbPath, os.Getenv("TOKEN_ANALYZER_DB"))
}

// Open 打开或创建 DB 文件，建表并设置 PRAGMA
func Open(path string) (*Database, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		// 回退到 data/
		fallback := filepath.Join("data", "token-analyzer.db")
		if err2 := os.MkdirAll(filepath.Dir(fallback), 0o755); err2 != nil {
			return nil, fmt.Errorf("创建 DB 目录失败: %w", err)
		}
		path = fallback
	}
	// modernc sqlite 需要 file: 前缀
	dsn := fmt.Sprintf("file:%s?cache=shared", path)
	// busy_timeout 通过 PRAGMA 设置
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// PRAGMA 顺序：auto_vacuum 必须在 journal_mode 之前且在建表前
	if _, err := sqlDB.Exec(`PRAGMA auto_vacuum = INCREMENTAL`); err != nil {
		// 忽略，已存在表时无法修改
	}
	if _, err := sqlDB.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		return nil, err
	}
	if _, err := sqlDB.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return nil, err
	}
	if _, err := sqlDB.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		return nil, err
	}
	if _, err := sqlDB.Exec(`PRAGMA synchronous = NORMAL`); err != nil {
		return nil, err
	}
	db := &Database{DB: sqlDB, Path: path}
	if err := migrate(sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// OpenReadOnly 以只读方式打开已存在的 ledger，供 Query 快照读取。
// 不建目录、不建表、不执行任何写 PRAGMA；文件缺失直接报错。
// query_only 兜底：Query 路径即便有 bug 也写不进 DB。
func OpenReadOnly(path string) (*Database, error) {
	// 路径经 URL 转义含入 URI：含空格/?/# 的路径也不走样。
	uri := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	sqlDB, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	if _, err := sqlDB.Exec(`PRAGMA query_only = ON`); err != nil {
		sqlDB.Close()
		return nil, err
	}
	if _, err := sqlDB.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		sqlDB.Close()
		return nil, err
	}
	// 缺失文件在这里现形：只读打开不存在的库要到首次访问才报错，
	// 提前碰一下 schema 给出明确错误（空 ledger 也是合法快照，不误报）。
	var tables int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM sqlite_master`).Scan(&tables); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("只读 ledger 不可用 %s: %w", path, err)
	}
	return &Database{DB: sqlDB, Path: path}, nil
}

func (d *Database) GetUserVersion() (int, error) {
	var v int
	if err := d.DB.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

func (d *Database) Close() error {
	return d.DB.Close()
}
