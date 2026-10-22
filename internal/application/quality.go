package application

import (
	"context"
	"time"
)

type QualityIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Detail   string `json:"detail"`
}
type QualityPage struct {
	Items  []QualityIssue   `json:"items"`
	Counts map[string]int64 `json:"counts"`
	Total  int64            `json:"total"`
	Page   int              `json:"page"`
}

const qualityCTE = `WITH db AS(SELECT d.*,s.name AS server_name,i.name AS instance_name,coalesce(dp.enabled,ip.enabled) AS policy_enabled FROM databases d LEFT JOIN instances i ON i.id=d.instance_id JOIN servers s ON s.id=coalesce(d.server_id,i.server_id) LEFT JOIN backup_policies dp ON dp.database_id=d.id LEFT JOIN backup_policies ip ON ip.instance_id=d.instance_id),issues AS(
 SELECT 'missing_responsible' AS code,'warning' AS severity,'database' AS kind,id,name,concat_ws(' / ',server_name,CASE WHEN dbms='Oracle Database' THEN 'Oracle Database' ELSE instance_name END,name) AS path,'Не указано ответственное лицо' AS detail FROM db WHERE trim(coalesce(attributes->>'responsible',''))=''
 UNION ALL SELECT 'missing_address','warning','server',id,name,name,'Не указан сетевой адрес сервера' FROM servers WHERE trim(address)=''
 UNION ALL SELECT 'production_no_policy','danger','database',id,name,concat_ws(' / ',server_name,instance_name,name),'Промышленная БД без политики РК' FROM db WHERE environment='Промышленный' AND policy_enabled IS NULL
 UNION ALL SELECT 'production_disabled_policy','warning','database',id,name,concat_ws(' / ',server_name,instance_name,name),'Эффективная политика РК промышленной БД отключена' FROM db WHERE environment='Промышленный' AND policy_enabled=false
 UNION ALL SELECT 'duplicate_server_name','warning','server',id,name,concat_ws(' • ',name,address),'Имена серверов совпадают без учёта регистра и крайних пробелов' FROM servers WHERE lower(trim(name)) IN(SELECT lower(trim(name)) FROM servers GROUP BY lower(trim(name)) HAVING count(*)>1)
 UNION ALL SELECT 'duplicate_server_address','warning','server',id,name,concat_ws(' • ',name,address),'Один адрес у нескольких серверов: возможны допустимые псевдонимы' FROM servers WHERE trim(address)<>'' AND lower(trim(address)) IN(SELECT lower(trim(address)) FROM servers WHERE trim(address)<>'' GROUP BY lower(trim(address)) HAVING count(*)>1)
 UNION ALL SELECT 'similar_sid','warning','database',d.id,d.name,concat_ws(' / ',d.server_name,d.instance_name,d.name),'SID различаются только регистром внутри одного родителя; проверьте допустимость' FROM db d WHERE EXISTS(SELECT 1 FROM db other WHERE other.id<>d.id AND coalesce(other.instance_id,other.server_id)=coalesce(d.instance_id,d.server_id) AND lower(other.name)=lower(d.name))
)`

func (s *Service) GetQualityReport(code string, page int) (QualityPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := QualityPage{Items: []QualityIssue{}, Counts: map[string]int64{}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	counts, err := tx.Query(ctx, qualityCTE+` SELECT code,count(*) FROM issues GROUP BY code`)
	if err != nil {
		return out, importDatabaseError(err)
	}
	for counts.Next() {
		var c string
		var n int64
		if err = counts.Scan(&c, &n); err != nil {
			counts.Close()
			return out, err
		}
		out.Counts[c] = n
		if code == "" || code == c {
			out.Total += n
		}
	}
	counts.Close()
	if err = counts.Err(); err != nil {
		return out, err
	}
	out.Page = boundedPage(page, out.Total, 25)
	rows, err := tx.Query(ctx, qualityCTE+` SELECT code,severity,kind,id::text,name,path,detail FROM issues WHERE $1='' OR code=$1 ORDER BY CASE severity WHEN 'danger' THEN 0 ELSE 1 END,code,lower(path),id LIMIT 25 OFFSET $2`, code, out.Page*25)
	if err != nil {
		return out, importDatabaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var item QualityIssue
		if err = rows.Scan(&item.Code, &item.Severity, &item.Kind, &item.ID, &item.Name, &item.Path, &item.Detail); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	return out, rows.Err()
}
