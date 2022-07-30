package springboard

import (
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"github.com/pkg/errors"
)

type SqliteRepo struct {
	db *sql.DB
}

// BoardCount implements BoardRepo
func (repo *SqliteRepo) BoardCount() (int, error) {
	query := `
		SELECT count(*)
		FROM boards
	`
	row := repo.db.QueryRow(query)

	var count int
	err := row.Scan(&count)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		} else {
			return 0, err
		}
	}

	return count, nil
}

// DeleteBoardsBefore implements BoardRepo
func (repo *SqliteRepo) DeleteBoardsBefore(expiry string) error {
	query := `
		  SELECT COUNT(*)
		  FROM boards
		  WHERE DATETIME(modified) < DATETIME(?)
		`
	row := repo.db.QueryRow(query, expiry)
	var count string
	err := row.Scan(&count)
	if err != nil {
		return errors.Wrap(err, "Error determining how many boards to delete")
	}
	log.Printf("  %s boards to delete", count)
	query = `
		  DELETE FROM boards
		  WHERE DATETIME(modified) < DATETIME(?)
		`
	_, err = repo.db.Exec(query, expiry)
	if err != nil {
		return errors.Wrap(err, "Error running deletion query")
	}
	return nil
}

// GetAllBoards implements BoardRepo
func (repo *SqliteRepo) GetAllBoards() ([]Board, error) {
	query := `
	  SELECT key, board, modified
	  FROM boards
	  ORDER BY modified DESC
	`
	rows, err := repo.db.Query(query)
	if err != nil {
		return nil, err
	}

	boards := []Board{}
	for rows.Next() {
		var key, board, modified string

		err = rows.Scan(&key, &board, &modified)
		if err != nil {
			return nil, err
		}

		modifiedTime, err := time.Parse(time.RFC3339, modified)
		if err != nil {
			return nil, err
		}

		boards = append(boards, Board{
			Key:      key,
			Board:    board,
			Modified: modifiedTime,
		})
	}

	return boards, nil
}

// GetBoard implements BoardRepo
func (repo *SqliteRepo) GetBoard(key string) (*Board, error) {
	query := `
		SELECT key, board, modified, signature
		FROM boards
		WHERE key=?
	`
	row := repo.db.QueryRow(query, key)

	var dbkey, board, modified, signature string
	err := row.Scan(&dbkey, &board, &modified, &signature)
	if err != nil {
		if err != sql.ErrNoRows {
			return nil, err
		}
		return nil, nil
	}

	modifiedTime, err := time.Parse(time.RFC3339, modified)
	if err != nil {
		return nil, err
	}

	return &Board{
		Key:       key,
		Board:     board,
		Modified:  modifiedTime,
		Signature: signature,
	}, nil
}

// PublishBoard implements BoardRepo
func (repo *SqliteRepo) PublishBoard(newBoard Board) error {
	_, err := repo.db.Exec(`
		INSERT INTO boards (key, board, modified, signature)
		            values(?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			    board=?,
			    modified=?,
			    signature=?
		`, newBoard.Key, newBoard.Board, newBoard.ModifiedAtDBFormat(), newBoard.Signature,
		newBoard.Board, newBoard.ModifiedAtDBFormat(), newBoard.Signature)
	if err != nil {
		return errors.Wrap(err, "Could not save board")
	} else {
		return nil
	}
}

func newSqliteRepo(dbName string) *SqliteRepo {
	// if the db doesn't exist, create it
	repo := SqliteRepo{}
	if _, err := os.Stat(dbName); errors.Is(err, os.ErrNotExist) {
		log.Printf("initializing new database")
		db, err := sql.Open("sqlite", dbName)
		if err != nil {
			panic(err)
		}

		initSQL := `
		CREATE TABLE boards (
			key text NOT NULL PRIMARY KEY,
			board text,
			modified text,
			signature test
		);
		CREATE INDEX boards_modified ON boards(modified);
		`

		_, err = db.Exec(initSQL)
		if err != nil {
			log.Fatalf("%q: %s\n", err, initSQL)
		}
		repo.db = db
	} else {
		db, err := sql.Open("sqlite", dbName)
		if err != nil {
			panic(err)
		}
		repo.db = db
	}
	return &repo
}
