package database

import "fmt"

type MigrationStateError struct {
	Version int64
	Reason  string
}

func (e *MigrationStateError) Error() string {
	return fmt.Sprintf("migration %d: %s", e.Version, e.Reason)
}
