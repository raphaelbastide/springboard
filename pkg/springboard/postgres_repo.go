package springboard

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/lib/pq"
	"github.com/pkg/errors"
)

type PostgresRepo struct {
	db *sql.DB
}

// BoardCount implements BoardRepo
func (repo *PostgresRepo) BoardCount() (int, error) {
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
func (repo *PostgresRepo) DeleteBoardsBefore(expiry string) error {
	query := `
		  SELECT COUNT(*)
		  FROM boards
		  WHERE modified < $1 
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
		  WHERE modified < $1
		`
	_, err = repo.db.Exec(query, expiry)
	if err != nil {
		return errors.Wrap(err, "Error running deletion query")
	}
	return nil
}

// GetAllBoards implements BoardRepo
func (repo *PostgresRepo) GetAllBoards() ([]Board, error) {
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
func (repo *PostgresRepo) GetBoard(key string) (*Board, error) {
	query := `
		SELECT key, board, modified, signature
		FROM boards
		WHERE key = $1
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
func (repo *PostgresRepo) PublishBoard(newBoard Board) error {
	_, err := repo.db.Exec(`
		INSERT INTO boards (key, board, modified, signature)
		            values($1, $2, $3, $4)
		ON CONFLICT(key) DO UPDATE SET
			    board=$2,
			    modified=$3,
			    signature=$4
		`, newBoard.Key, newBoard.Board, newBoard.ModifiedAtDBFormat(), newBoard.Signature)
	if err != nil {
		return errors.Wrap(err, "Could not save board")
	} else {
		return nil
	}
}

func newPostgresRepo(dbName string) *PostgresRepo {
	// if the db doesn't exist, create it
	repo := PostgresRepo{}
	db, err := sql.Open("postgres", dbName)
	if err != nil {
		panic(err)
	}

	initSQL := `
	CREATE TABLE IF NOT EXISTS boards (
		key VARCHAR(64) NOT NULL PRIMARY KEY,
		board VARCHAR(2217),
		modified TIMESTAMP,
		signature VARCHAR(128)
	);
	CREATE INDEX IF NOT EXISTS boards_modified ON boards(modified);
	`

	_, err = db.Exec(initSQL)
	if err != nil {
		log.Fatalf("%q: %s\n", err, initSQL)
	}
	repo.db = db
	return &repo
}
