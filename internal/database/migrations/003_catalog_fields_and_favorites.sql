ALTER TABLE servers ADD COLUMN attributes jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(attributes)='object');
ALTER TABLE instances ADD COLUMN attributes jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(attributes)='object');
ALTER TABLE databases ADD COLUMN attributes jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(attributes)='object');
ALTER TABLE instances ADD COLUMN environment text NOT NULL DEFAULT '';
ALTER TABLE databases ADD COLUMN environment text NOT NULL DEFAULT '';
UPDATE servers SET status='Эксплуатируется',version=version+1,updated_at=now() WHERE status IN ('Активен','Активна');
UPDATE instances SET status='Эксплуатируется',version=version+1,updated_at=now() WHERE status IN ('Активен','Активна');
UPDATE databases SET status='Эксплуатируется',version=version+1,updated_at=now() WHERE status IN ('Активен','Активна');
ALTER TABLE servers ALTER COLUMN status SET DEFAULT 'Эксплуатируется';
ALTER TABLE instances ALTER COLUMN status SET DEFAULT 'Эксплуатируется';
ALTER TABLE databases ALTER COLUMN status SET DEFAULT 'Эксплуатируется';
CREATE TABLE lookup_values (
 category text NOT NULL,
 value text NOT NULL CHECK(length(trim(value))>0),
 position integer NOT NULL DEFAULT 0,
 PRIMARY KEY(category,value)
);
INSERT INTO lookup_values(category,value,position) VALUES
 ('status','Эксплуатируется',1),('status','Выведена из эксплуатации',2),('status','Заморожена',3),('status','Устаревшая',4),('status','Справочник',5),('status','К удалению',6),('status','Актуализировать',7),('status','Архив',8),('status','Разрабатывается',9),('status','Отсоединена',10),
 ('environment','Тестовый',1),('environment','Промышленный',2),
 ('dbms','PostgreSQL',1),('dbms','Oracle Database',2),('dbms','MySQL/MariaDB',3),('dbms','Firebird',4),('dbms','Microsoft SQL Server',5),
 ('domain','oms.admsurgut.ru',1),('domain','отсутствует',2),
 ('connectionMethod','RDP',1),('connectionMethod','SSH',2),
 ('location','МКУ "УИТС г. Сургута"',1),('serviceOrganization','МКУ "УИТС г. Сургута"',1),
 ('backupType','Локальное',1),('backupType','Централизованное',2),('backupType','Локальное и Централизованное',3),
 ('administration','Полное администрирование',1),('administration','Частичное сопровождение',2),('administration','Консультационная поддержка',3),('administration','Мониторинг и контроль',4),('administration','Резервное копирование',5),('administration','Управление пользователями и ролями',6),('administration','Восстановление БД по запросу',7),('administration','Аудит и безопасность БД',8),('administration','Без участия АБД',9);
CREATE TABLE favorites (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 server_id uuid REFERENCES servers(id) ON DELETE CASCADE,
 instance_id uuid REFERENCES instances(id) ON DELETE CASCADE,
 database_id uuid REFERENCES databases(id) ON DELETE CASCADE,
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(num_nonnulls(server_id,instance_id,database_id)=1),
 UNIQUE(user_id,server_id),UNIQUE(user_id,instance_id),UNIQUE(user_id,database_id)
);
CREATE INDEX favorites_user ON favorites(user_id);
