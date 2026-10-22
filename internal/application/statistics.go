package application

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

type Distribution struct {
	Name      string `json:"name"`
	Instances int64  `json:"instances"`
	Databases int64  `json:"databases"`
}
type Statistics struct {
	Servers              int64          `json:"servers"`
	UniqueServers        int64          `json:"uniqueServers"`
	ServerAddresses      int64          `json:"serverAddresses"`
	Instances            int64          `json:"instances"`
	Databases            int64          `json:"databases"`
	Policies             int64          `json:"policies"`
	EnabledPolicies      int64          `json:"enabledPolicies"`
	ProtectedDatabases   int64          `json:"protectedDatabases"`
	UnprotectedDatabases int64          `json:"unprotectedDatabases"`
	DisabledDatabases    int64          `json:"disabledDatabases"`
	DBMS                 []Distribution `json:"dbms"`
	Statuses             []Distribution `json:"statuses"`
	Environments         []Distribution `json:"environments"`
	GeneratedAt          time.Time      `json:"generatedAt"`
}

func (s *Service) GetStatistics() (Statistics, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return Statistics{}, err
	}
	defer tx.Rollback(ctx)
	return loadStatistics(ctx, tx)
}
func loadStatistics(ctx context.Context, tx pgx.Tx) (Statistics, error) {
	out := Statistics{DBMS: []Distribution{}, Statuses: []Distribution{}, Environments: []Distribution{}, GeneratedAt: time.Now().UTC()}
	err := tx.QueryRow(ctx, `SELECT count(*),count(DISTINCT lower(trim(name))),count(DISTINCT (lower(trim(name)),lower(trim(address)))) FROM servers`).Scan(&out.Servers, &out.UniqueServers, &out.ServerAddresses)
	if err != nil {
		return out, importDatabaseError(err)
	}
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM instances),(SELECT count(*) FROM databases),(SELECT count(*) FROM backup_policies),(SELECT count(*) FROM backup_policies WHERE enabled)`).Scan(&out.Instances, &out.Databases, &out.Policies, &out.EnabledPolicies); err != nil {
		return out, importDatabaseError(err)
	}
	// An explicitly disabled database policy overrides its instance policy.
	if err = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE coalesce(dp.enabled,ip.enabled,false)),count(*) FILTER(WHERE dp.id IS NULL AND ip.id IS NULL),count(*) FILTER(WHERE NOT coalesce(dp.enabled,ip.enabled,true)) FROM databases d LEFT JOIN backup_policies dp ON dp.database_id=d.id LEFT JOIN backup_policies ip ON ip.instance_id=d.instance_id`).Scan(&out.ProtectedDatabases, &out.UnprotectedDatabases, &out.DisabledDatabases); err != nil {
		return out, importDatabaseError(err)
	}
	for _, group := range []struct {
		query string
		dst   *[]Distribution
	}{
		{`SELECT dbms,sum(instances)::bigint,sum(databases)::bigint FROM (SELECT i.dbms,count(DISTINCT i.id) AS instances,count(d.id) AS databases FROM instances i LEFT JOIN databases d ON d.instance_id=i.id GROUP BY i.dbms UNION ALL SELECT dbms,0,count(*) FROM databases WHERE server_id IS NOT NULL GROUP BY dbms) t GROUP BY dbms ORDER BY sum(databases) DESC,dbms`, &out.DBMS},
		{`SELECT status,0,count(*) FROM databases GROUP BY status ORDER BY count(*) DESC,status`, &out.Statuses},
		{`SELECT environment,0,count(*) FROM databases GROUP BY environment ORDER BY count(*) DESC,environment`, &out.Environments},
	} {
		rows, e := tx.Query(ctx, group.query)
		if e != nil {
			return out, importDatabaseError(e)
		}
		for rows.Next() {
			var item Distribution
			if e = rows.Scan(&item.Name, &item.Instances, &item.Databases); e != nil {
				rows.Close()
				return out, importDatabaseError(e)
			}
			*group.dst = append(*group.dst, item)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return out, importDatabaseError(e)
		}
	}
	return out, nil
}
