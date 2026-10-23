package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type PolicyImportOverride struct {
	Row    int    `json:"row"`
	Skip   bool   `json:"skip"`
	Policy Policy `json:"policy"`
}
type PolicyImportConfig struct {
	Enabled       bool                   `json:"enabled"`
	TargetKind    string                 `json:"targetKind"`
	DefaultState  string                 `json:"defaultState"`
	RetentionUnit string                 `json:"retentionUnit"`
	Defaults      Policy                 `json:"defaults"`
	Overrides     []PolicyImportOverride `json:"overrides"`
}
type ImportPolicyCandidate struct {
	Row     int      `json:"row"`
	Target  string   `json:"target"`
	Action  string   `json:"action"`
	Policy  Policy   `json:"policy"`
	Before  *Policy  `json:"before"`
	Errors  []string `json:"errors"`
	Skipped bool     `json:"skipped"`
}
type plannedImportPolicy struct {
	key    string
	policy Policy
	rows   []int
}

func importPolicyStamp(ctx context.Context, tx pgx.Tx) (string, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT coalesce(jsonb_agg(p ORDER BY id),'[]') FROM backup_policies p`).Scan(&raw)
	return fmt.Sprintf("%x", sha256.Sum256(raw)), err
}
func (s *Service) PreviewExcelPolicies(token string, c PolicyImportConfig) (ImportPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.pendingImport
	if old == nil || old.preview.Token != token || time.Now().After(old.expires) || s.user == nil || old.userID != s.user.ID {
		return ImportPreview{}, errors.New("Предпросмотр устарел")
	}
	if old.preview.Columns != 28 {
		return ImportPreview{}, errors.New("Параметры РК доступны для формата 28 колонок")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, true, false)
	if err != nil {
		return ImportPreview{}, err
	}
	defer tx.Rollback(ctx)
	next, err := s.buildImportPlanExcluded(ctx, tx, old.workbook, old.excluded)
	if err != nil {
		return ImportPreview{}, err
	}
	if c.Enabled {
		if err = s.attachImportPolicies(ctx, tx, next, c); err != nil {
			return ImportPreview{}, err
		}
	}
	s.pendingImport = next
	return next.preview, nil
}
func (s *Service) attachImportPolicies(ctx context.Context, tx pgx.Tx, plan *importPlan, c PolicyImportConfig) error {
	if c.TargetKind != "database" && c.TargetKind != "instance" {
		return errors.New("Явно выберите уровень назначения: БД или экземпляр")
	}
	if c.DefaultState != "" && c.DefaultState != "enabled" && c.DefaultState != "disabled" {
		return errors.New("Неизвестное состояние политики по умолчанию")
	}
	if c.RetentionUnit != "" && c.RetentionUnit != "days" && c.RetentionUnit != "weeks" {
		return errors.New("Выберите единицу срока хранения: дни или недели")
	}
	overrides := map[int]PolicyImportOverride{}
	available := map[int]bool{}
	for _, r := range plan.workbook.Rows {
		available[r.Number] = true
	}
	for _, o := range c.Overrides {
		if !available[o.Row] {
			return errors.New("Неизвестная строка исправления политики")
		}
		if _, ok := overrides[o.Row]; ok {
			return errors.New("Повтор исправления политики")
		}
		overrides[o.Row] = o
	}
	plan.policyConfig = &c
	plan.preview.WithPolicies = true
	plan.preview.Policies = []ImportPolicyCandidate{}
	var err error
	plan.policyStamp, err = importPolicyStamp(ctx, tx)
	if err != nil {
		return err
	}
	choices, err := loadChoices(ctx, tx)
	if err != nil {
		return err
	}
	idx, err := loadIndex(ctx, tx, plan.workbook)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT to_jsonb(p) FROM backup_policies p ORDER BY id`)
	if err != nil {
		return err
	}
	existing := map[string]Policy{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		p, e := decodePolicy(raw)
		if e != nil {
			rows.Close()
			return e
		}
		existing[p.Kind+":"+p.ObjectID] = p
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	source := map[int][26]string{}
	for _, r := range plan.workbook.Rows {
		source[r.Number] = r.Values
	}
	groups := map[string]int{}
	candidateKeys := []string{}
	for _, item := range plan.items {
		v := source[item.number]
		o, overridden := overrides[item.number]
		has := false
		for _, x := range v[21:26] {
			if x != "" {
				has = true
			}
		}
		if !has && !overridden {
			continue
		}
		candidate := ImportPolicyCandidate{Row: item.number, Errors: []string{}, Target: item.server.Name + " / " + item.instance.Name + " / " + item.database.Name, Action: "создать", Skipped: o.Skip}
		p := Policy{Name: "РК: " + item.database.Name, Kind: c.TargetKind, BackupType: v[21], Storage: v[22], Scheduler: v[23], Schedule: c.Defaults.Schedule, RetentionDays: c.Defaults.RetentionDays, Description: c.Defaults.Description}
		for _, f := range [][2]*string{{&p.BackupType, &c.Defaults.BackupType}, {&p.Storage, &c.Defaults.Storage}, {&p.Scheduler, &c.Defaults.Scheduler}} {
			if *f[0] == "" {
				*f[0] = *f[1]
			}
		}
		if p.Kind == "instance" {
			p.Name = "РК: " + item.instance.Name
		}
		if overridden {
			p = o.Policy
		} else {
			state := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(v[24])), "ё", "е")
			switch state {
			case "активен", "включен", "активна", "включена", "включено", "работает", "да", "enabled", "active", "1":
				p.Enabled = true
			case "отключен", "отключена", "отключено", "нет", "disabled", "inactive", "0":
				p.Enabled = false
			case "":
				if c.DefaultState == "" {
					candidate.Errors = append(candidate.Errors, "Выберите состояние для пустого статуса СРК")
				} else {
					p.Enabled = c.DefaultState == "enabled"
				}
			default:
				candidate.Errors = append(candidate.Errors, "Неизвестный статус СРК: "+v[24]+"; исправьте политику")
			}
			retention := strings.ToLower(strings.TrimSpace(v[25]))
			if retention != "" {
				parts := strings.Fields(retention)
				n, e := strconv.Atoi(parts[0])
				unit := c.RetentionUnit
				if len(parts) > 1 {
					if strings.HasPrefix(parts[1], "дн") || strings.HasPrefix(parts[1], "день") || parts[1] == "дней" || parts[1] == "суток" || parts[1] == "сутки" {
						unit = "days"
					} else if strings.HasPrefix(parts[1], "нед") {
						unit = "weeks"
					} else {
						unit = ""
					}
				}
				if e != nil || unit == "" || len(parts) > 2 || n < 0 || n > 36500 || (unit == "weeks" && n > 36500/7) {
					candidate.Errors = append(candidate.Errors, "Уточните срок хранения и его единицу")
				} else {
					if unit == "weeks" {
						n *= 7
					}
					p.RetentionDays = n
				}
			}
		}
		_, ik, dk := item.keys()
		key := dk
		if p.Kind == "instance" {
			key = ik
			candidate.Target = item.server.Name + " / " + item.instance.Name
			if item.isOracle() {
				candidate.Errors = append(candidate.Errors, "Oracle Database: политика назначается БД, без экземпляра")
			}
		}
		p.ID = ""
		p.Version = 0
		p.ObjectID = idx.get(key).ID
		if p.ObjectID == "" {
			p.ObjectID = "pending"
		}
		if !o.Skip {
			normalized, e := normalizePolicy(p)
			if e != nil {
				candidate.Errors = append(candidate.Errors, e.Error())
			} else {
				p = normalized
				if p.BackupType != "Не указано" {
					if e = choices.validate("backupType", p.BackupType, ""); e != nil {
						candidate.Errors = append(candidate.Errors, e.Error())
					}
				}
			}
		}
		if old, ok := existing[p.Kind+":"+p.ObjectID]; ok {
			copy := old
			candidate.Before = &copy
			candidate.Action = "обновить"
			p.ID = old.ID
			p.Version = old.Version
		}
		candidate.Policy = p
		plan.preview.Policies = append(plan.preview.Policies, candidate)
		candidateKeys = append(candidateKeys, key)
		if o.Skip {
			continue
		}
		if at, ok := groups[key]; ok {
			previous := &plan.policies[at]
			if !reflect.DeepEqual(previous.policy, p) {
				candidate.Errors = append(candidate.Errors, "Одна цель РК содержит разные параметры в нескольких строках")
				plan.preview.Policies[len(plan.preview.Policies)-1].Errors = candidate.Errors
				for i, k := range candidateKeys {
					if k == key && !plan.preview.Policies[i].Skipped {
						plan.preview.Policies[i].Errors = append(plan.preview.Policies[i].Errors, "Конфликт параметров одной политики РК")
					}
				}
			}
			previous.rows = append(previous.rows, item.number)
		} else {
			groups[key] = len(plan.policies)
			plan.policies = append(plan.policies, plannedImportPolicy{key, p, []int{item.number}})
		}
	}
	errorsByRow := map[int][]string{}
	for _, c := range plan.preview.Policies {
		if !c.Skipped {
			errorsByRow[c.Row] = append(errorsByRow[c.Row], c.Errors...)
		}
	}
	plan.preview.Valid = 0
	for i := range plan.items {
		item := &plan.items[i]
		item.errors = append(item.errors, errorsByRow[item.number]...)
		plan.preview.Rows[i].Errors = item.errors
		if len(item.errors) == 0 {
			plan.preview.Valid++
		}
	}
	return nil
}
