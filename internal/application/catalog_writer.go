package application

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// catalogWriter shares transaction-scoped validation across single and batch writes.
// Authorization and commit remain the caller's responsibility.
type catalogWriter struct {
	service    *Service
	tx         pgx.Tx
	choices    choiceSet
	moveReason string
}

func (s *Service) catalogWriter(ctx context.Context, tx pgx.Tx) (catalogWriter, error) {
	choices, err := loadChoices(ctx, tx)
	return catalogWriter{service: s, tx: tx, choices: choices}, err
}

type writeMode uint8

const (
	writeNormal writeMode = iota
	writeRestoreVersion
	writeRecreateDeleted
)

func versionWriteMode(restore bool) writeMode {
	if restore {
		return writeRestoreVersion
	}
	return writeNormal
}
