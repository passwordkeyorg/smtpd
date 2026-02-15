package spool

import (
	"crypto/rand"
	"time"

	"github.com/oklog/ulid/v2"
)

func NewULID(t time.Time) string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return ulid.MustNew(ulid.Timestamp(t), entropy).String()
}
