package springboard

import "time"

type Board struct {
	Key       string
	Board     string
	Modified  time.Time
	Signature string
}

func (board Board) ModifiedAtDBFormat() string {
	return board.Modified.Format(time.RFC3339)
}
