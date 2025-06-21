package db

import (
	"database/sql"
	"errors"

	_ "github.com/mattn/go-sqlite3"
)

// User — представление строки из users
type User struct {
	ID      int
	PeerID  string
	Tokens  int
	Enabled bool
	Mode    string
}

var Conn *sql.DB

// Init открывает БД и создаёт таблицу, если нужно.
func Init(dsn string) error {
	var err error
	Conn, err = sql.Open("sqlite3", dsn)
	if err != nil {
		return err
	}
	// Ждём, пока БД инициализируется
	if err := Conn.Ping(); err != nil {
		return err
	}

	// Создаём таблицу users
	_, err = Conn.Exec(`
        CREATE TABLE IF NOT EXISTS users (
            id      INTEGER PRIMARY KEY AUTOINCREMENT,
            peer_id TEXT    UNIQUE NOT NULL,
            tokens  INTEGER NOT NULL DEFAULT 0,
            mode    TEXT    NOT NULL DEFAULT 'turned_off',
        );
    `)
	return err
}

// ensureRow создаёт строку с нулевыми значениями, если её нет
func ensureRow(peerID string) error {
	_, err := Conn.Exec(
		`INSERT OR IGNORE INTO users(peer_id) VALUES(?)`,
		peerID,
	)
	return err
}

// GetUser возвращает все поля пользователя
func GetUser(peerID string) (*User, error) {
	if err := ensureRow(peerID); err != nil {
		return nil, err
	}
	row := Conn.QueryRow(
		`SELECT id, peer_id, tokens, enabled, mode FROM users WHERE peer_id = ?`,
		peerID,
	)
	u := &User{}
	var enabledInt int
	if err := row.Scan(&u.ID, &u.PeerID, &u.Tokens, &enabledInt, &u.Mode); err != nil {
		return nil, err
	}
	u.Enabled = enabledInt != 0
	return u, nil
}

// ChangeTokens изменяет баланс токенов на delta (можно отрицательное число).
// Возвращает новый баланс или ошибку.
func ChangeTokens(peerID string, delta int) (int, error) {
	if err := ensureRow(peerID); err != nil {
		return 0, err
	}
	// Применяем изменение
	_, err := Conn.Exec(
		`UPDATE users SET tokens = tokens + ? WHERE peer_id = ?`,
		delta, peerID,
	)
	if err != nil {
		return 0, err
	}
	// Читаем обновлённое значение
	row := Conn.QueryRow(
		`SELECT tokens FROM users WHERE peer_id = ?`,
		peerID,
	)
	var tokens int
	if err := row.Scan(&tokens); err != nil {
		return 0, err
	}
	return tokens, nil
}

// SetEnabled включает или отключает пользователя
func SetEnabled(peerID string, enabled bool) error {
	if err := ensureRow(peerID); err != nil {
		return err
	}
	e := 0
	if enabled {
		e = 1
	}
	_, err := Conn.Exec(
		`UPDATE users SET enabled = ? WHERE peer_id = ?`,
		e, peerID,
	)
	return err
}

// SetMode меняет режим работы пользователя ("initiator", "processor", "all")
func SetMode(peerID, mode string) error {
	if err := ensureRow(peerID); err != nil {
		return err
	}
	if mode != "initiator" && mode != "processor" && mode != "all" {
		return errors.New("invalid mode")
	}
	_, err := Conn.Exec(
		`UPDATE users SET mode = ? WHERE peer_id = ?`,
		mode, peerID,
	)
	return err
}
