package application

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type Notification struct {
	ID        string     `json:"id"`
	Category  string     `json:"category"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Entity    string     `json:"entity"`
	ObjectID  string     `json:"objectId"`
	CreatedAt time.Time  `json:"createdAt"`
	ReadAt    *time.Time `json:"readAt"`
}
type NotificationPage struct {
	Items  []Notification `json:"items"`
	Unread int64          `json:"unread"`
	Total  int64          `json:"total"`
	Page   int            `json:"page"`
}

func notificationEvent(ctx context.Context, tx pgx.Tx, actor, action, entity, id string) error {
	var err error
	switch action {
	case "register":
		_, err = tx.Exec(ctx, `INSERT INTO notifications(user_id,category,title,body,entity,object_id) SELECT a.id,'registration','Заявка на регистрацию',u.login||' ожидает подтверждения','user',u.id FROM users a CROSS JOIN users u WHERE a.role='admin' AND a.state='active' AND u.id=$1 AND u.state='pending'`, id)
	case "user_password_change":
		_, err = tx.Exec(ctx, `INSERT INTO notifications(user_id,category,title,body,entity,object_id) SELECT id,'account','Пароль изменён','Администратор изменил пароль вашей учётной записи. Новый пароль применяется при следующем входе.','account',id FROM users WHERE id=$1`, id)
	case "user_update":
		_, err = tx.Exec(ctx, `INSERT INTO notifications(user_id,category,title,body,entity,object_id) SELECT id,'account','Изменена учётная запись','Роль: '||CASE role WHEN 'admin' THEN 'Администратор' WHEN 'editor' THEN 'Редактор' ELSE 'Наблюдатель' END||'. Состояние: '||CASE state WHEN 'active' THEN 'Активна' WHEN 'pending' THEN 'Ожидает подтверждения' WHEN 'blocked' THEN 'Заблокирована' ELSE 'Отключена' END,'account',id FROM users WHERE id=$1`, id)
	case "update", "delete", "restore":
		if entity != "server" && entity != "instance" && entity != "database" {
			return nil
		}
		title := "Изменён избранный объект"
		if action == "delete" {
			title = "Удалён избранный объект"
		}
		_, err = tx.Exec(ctx, `INSERT INTO notifications(user_id,category,title,body,entity,object_id) SELECT f.user_id,'favorite',$4,coalesce((SELECT coalesce(after_state->>'name',before_state->>'name') FROM object_history WHERE entity=$2 AND object_id=$3 ORDER BY id DESC LIMIT 1),'Объект каталога'),$2,$3 FROM favorites f JOIN users u ON u.id=f.user_id WHERE (CASE $2 WHEN 'server' THEN f.server_id WHEN 'instance' THEN f.instance_id ELSE f.database_id END)=$3::uuid AND f.user_id<>$1 AND u.state='active'`, actor, entity, id, title)
	}
	return err
}
func (s *Service) GetNotifications(page int, unreadOnly bool) (NotificationPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := NotificationPage{Items: []Notification{}}
	if page < 0 {
		page = 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE read_at IS NULL) FROM notifications WHERE user_id=$1`, s.user.ID).Scan(&out.Total, &out.Unread); err != nil {
		return out, importDatabaseError(err)
	}
	if unreadOnly {
		out.Total = out.Unread
	}
	if page > int(out.Total/25) {
		page = int(out.Total / 25)
	}
	if out.Total > 0 && int64(page*25) >= out.Total {
		page--
	}
	out.Page = page
	rows, err := tx.Query(ctx, `SELECT id::text,category,title,body,entity,coalesce(object_id::text,''),created_at,read_at FROM notifications WHERE user_id=$1 AND (NOT $2 OR read_at IS NULL) ORDER BY id DESC LIMIT 25 OFFSET $3`, s.user.ID, unreadOnly, page*25)
	if err != nil {
		return out, importDatabaseError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var n Notification
		if err = rows.Scan(&n.ID, &n.Category, &n.Title, &n.Body, &n.Entity, &n.ObjectID, &n.CreatedAt, &n.ReadAt); err != nil {
			return out, importDatabaseError(err)
		}
		out.Items = append(out.Items, n)
	}
	return out, rows.Err()
}
func (s *Service) MarkNotificationsRead(ids []string, all bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !all && (len(ids) == 0 || len(ids) > 100) {
		return errors.New("Выберите от 1 до 100 уведомлений")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := s.readTransaction(ctx, false, false)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE notifications SET read_at=now() WHERE user_id=$1 AND read_at IS NULL AND ($2 OR id::text=ANY($3::text[]))`, s.user.ID, all, ids); err != nil {
		return importDatabaseError(err)
	}
	return tx.Commit(ctx)
}
