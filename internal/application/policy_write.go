package application

import (
	"context"
	"encoding/json"
)

func (w catalogWriter) savePolicy(ctx context.Context, p Policy, mode writeMode) (Policy, error) {
	var err error
	p, err = normalizePolicy(p)
	if err != nil {
		return p, err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	var result []byte
	if err = w.tx.QueryRow(ctx, `SELECT api_save_record('policy',$1,$2,$3,'')`, raw, p.Version, int(mode)).Scan(&result); err != nil {
		return p, importDatabaseError(err)
	}
	err = json.Unmarshal(result, &p)
	return p, err
}
