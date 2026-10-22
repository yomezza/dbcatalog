CREATE TABLE notifications (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 category text NOT NULL,
 title text NOT NULL,
 body text NOT NULL DEFAULT '',
 entity text NOT NULL DEFAULT '',
 object_id uuid,
 created_at timestamptz NOT NULL DEFAULT now(),
 read_at timestamptz
);
CREATE INDEX notifications_inbox ON notifications(user_id,id DESC);
CREATE INDEX notifications_unread ON notifications(user_id,id DESC) WHERE read_at IS NULL;
