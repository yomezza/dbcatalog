package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var actionLabels = map[string]string{"pdf_export": "Формирование PDF-паспорта", "user_password_change": "Смена пароля пользователя", "attachment_add": "Добавление вложения", "attachment_delete": "Удаление вложения", "instruction_update": "Обновление инструкции", "object_move": "Перемещение объекта", "bulk_update": "Массовое редактирование БД", "version_restore": "Возврат к версии", "oracle_migration": "Переход Oracle на прямое размещение", "create": "Создание", "update": "Изменение", "delete": "Удаление", "restore": "Восстановление", "login": "Вход", "login_failed": "Неудачный вход", "register": "Регистрация", "user_update": "Изменение пользователя", "favorite_add": "Добавление в избранное", "favorite_remove": "Удаление из избранного", "excel_import": "Импорт Excel", "excel_export": "Экспорт Excel", "cascade_delete": "Удаление ветви каталога", "catalog_clear": "Очистка каталога"}

type AuditFilter struct {
	Query    string `json:"query"`
	ActorID  string `json:"actorId"`
	Action   string `json:"action"`
	Entity   string `json:"entity"`
	From     string `json:"from"`
	To       string `json:"to"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}
type AuditEvent struct {
	ID          string         `json:"id"`
	ActorID     string         `json:"actorId"`
	Actor       string         `json:"actor"`
	Action      string         `json:"action"`
	ActionLabel string         `json:"actionLabel"`
	Entity      string         `json:"entity"`
	ObjectID    string         `json:"objectId"`
	ObjectName  string         `json:"objectName"`
	CreatedAt   time.Time      `json:"createdAt"`
	Details     map[string]any `json:"details"`
}
type AuditActor struct {
	ID    string `json:"id"`
	Login string `json:"login"`
}
type AuditPage struct {
	Events   []AuditEvent      `json:"events"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"pageSize"`
	Actors   []AuditActor      `json:"actors"`
	Actions  map[string]string `json:"actions"`
}

func (s *Service) GetAudit(filter AuditFilter) (AuditPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := AuditPage{Events: []AuditEvent{}, Actors: []AuditActor{}, Actions: map[string]string{}}
	if filter.Page < 0 {
		filter.Page = 0
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 25
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	if len(filter.Query) > 2000 {
		return result, errors.New("Поисковая строка слишком длинная")
	}
	terms := []string{}
	for _, v := range strings.Split(filter.Query, "|") {
		if v = strings.TrimSpace(v); v != "" {
			terms = append(terms, v)
		}
	}
	if len(terms) > 20 {
		return result, errors.New("Допускается не более 20 условий поиска")
	}
	var from, to time.Time
	var err error
	if filter.From != "" {
		from, err = time.Parse(time.RFC3339, filter.From)
		if err != nil {
			return result, errors.New("Некорректная начальная дата")
		}
	}
	if filter.To != "" {
		to, err = time.Parse(time.RFC3339, filter.To)
		if err != nil {
			return result, errors.New("Некорректная конечная дата")
		}
	}
	if !from.IsZero() && !to.IsZero() && !from.Before(to) {
		return result, errors.New("Начало периода должно предшествовать окончанию")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, true)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	labels, _ := json.Marshal(actionLabels)
	args := []any{labels}
	where := []string{"true"}
	add := func(condition string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(condition, len(args)))
	}
	if filter.ActorID != "" {
		add("actor_id=$%d::uuid", filter.ActorID)
	}
	if filter.Action != "" {
		add("action=$%d", filter.Action)
	}
	if filter.Entity != "" {
		add("entity=$%d", filter.Entity)
	}
	if !from.IsZero() {
		add("created_at >= $%d", from)
	}
	if !to.IsZero() {
		add("created_at < $%d", to)
	}
	for _, term := range terms {
		add("position(lower($%d) in lower(concat_ws(' ',id,actor,action,action_label,entity,object_id,object_name,details::text)))>0", term)
	}
	cte := `WITH events AS (SELECT a.id::text AS id,a.id AS sort_id,a.actor_id,COALESCE(u.login,'Не определён') AS actor,a.action,COALESCE($1::jsonb->>a.action,a.action) AS action_label,a.entity,COALESCE(a.object_id::text,'') AS object_id,COALESCE(a.details->>'objectName',h.name,'') AS object_name,a.created_at,a.details FROM audit_events a LEFT JOIN users u ON u.id=a.actor_id LEFT JOIN LATERAL (SELECT COALESCE(after_state->>'name',before_state->>'name') AS name FROM object_history WHERE entity=a.entity AND object_id=a.object_id ORDER BY id DESC LIMIT 1) h ON NOT(a.details?'objectName')) `
	conditions := strings.Join(where, " AND ")
	if err = tx.QueryRow(ctx, cte+"SELECT count(*) FROM events WHERE "+conditions, args...).Scan(&result.Total); err != nil {
		return result, importDatabaseError(err)
	}
	last := 0
	if result.Total > 0 {
		last = int((result.Total - 1) / int64(filter.PageSize))
	}
	if filter.Page > last {
		filter.Page = last
	}
	result.Page = filter.Page
	result.PageSize = filter.PageSize
	args = append(args, filter.PageSize, int64(filter.Page)*int64(filter.PageSize))
	rows, err := tx.Query(ctx, cte+"SELECT id,COALESCE(actor_id::text,''),actor,action,action_label,entity,object_id,object_name,created_at,details FROM events WHERE "+conditions+fmt.Sprintf(" ORDER BY sort_id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return result, importDatabaseError(err)
	}
	for rows.Next() {
		var e AuditEvent
		if err = rows.Scan(&e.ID, &e.ActorID, &e.Actor, &e.Action, &e.ActionLabel, &e.Entity, &e.ObjectID, &e.ObjectName, &e.CreatedAt, &e.Details); err != nil {
			rows.Close()
			return result, importDatabaseError(err)
		}
		result.Events = append(result.Events, e)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, importDatabaseError(err)
	}
	rows, err = tx.Query(ctx, `SELECT id::text,login FROM users ORDER BY lower(login),id`)
	if err != nil {
		return result, importDatabaseError(err)
	}
	for rows.Next() {
		var a AuditActor
		if err = rows.Scan(&a.ID, &a.Login); err != nil {
			rows.Close()
			return result, importDatabaseError(err)
		}
		result.Actors = append(result.Actors, a)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, importDatabaseError(err)
	}
	rows, err = tx.Query(ctx, `SELECT DISTINCT action FROM audit_events ORDER BY action`)
	if err != nil {
		return result, importDatabaseError(err)
	}
	for rows.Next() {
		var action string
		if err = rows.Scan(&action); err != nil {
			rows.Close()
			return result, importDatabaseError(err)
		}
		label := actionLabels[action]
		if label == "" {
			label = action
		}
		result.Actions[action] = label
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, importDatabaseError(err)
	}
	return result, nil
}
