CREATE FUNCTION catalog_audit_notifications() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE title text;objname text;
BEGIN
 CASE NEW.action
 WHEN 'register' THEN
  INSERT INTO notifications(user_id,category,title,body,entity,object_id)
   SELECT a.id,'registration','Заявка на регистрацию',u.login||' ожидает подтверждения','user',u.id
   FROM users a CROSS JOIN users u WHERE a.role='admin' AND a.state='active' AND u.id=NEW.object_id AND u.state='pending';
 WHEN 'user_password_change' THEN
  INSERT INTO notifications(user_id,category,title,body,entity,object_id)
   SELECT id,'account','Пароль изменён','Администратор изменил пароль вашей учётной записи. Новый пароль применяется при следующем входе.','account',id FROM users WHERE id=NEW.object_id;
 WHEN 'user_update' THEN
  INSERT INTO notifications(user_id,category,title,body,entity,object_id)
   SELECT id,'account','Изменена учётная запись','Роль: '||CASE role WHEN 'admin' THEN 'Администратор' WHEN 'editor' THEN 'Редактор' ELSE 'Наблюдатель' END||
   '. Состояние: '||CASE state WHEN 'active' THEN 'Активна' WHEN 'pending' THEN 'Ожидает подтверждения' WHEN 'blocked' THEN 'Заблокирована' ELSE 'Отключена' END,'account',id FROM users WHERE id=NEW.object_id;
 ELSE
  IF NEW.action IN('update','delete','restore') AND NEW.entity IN('server','instance','database') THEN
   title:=CASE WHEN NEW.action='delete' THEN 'Удалён избранный объект' ELSE 'Изменён избранный объект' END;
   objname:=coalesce(NEW.details->>'objectName',(SELECT coalesce(after_state->>'name',before_state->>'name') FROM object_history WHERE entity=NEW.entity AND object_id=NEW.object_id ORDER BY id DESC LIMIT 1),'Объект каталога');
   INSERT INTO notifications(user_id,category,title,body,entity,object_id)
    SELECT f.user_id,'favorite',title,objname,NEW.entity,NEW.object_id FROM favorites f JOIN users u ON u.id=f.user_id
    WHERE(CASE NEW.entity WHEN 'server' THEN f.server_id WHEN 'instance' THEN f.instance_id ELSE f.database_id END)=NEW.object_id
    AND f.user_id IS DISTINCT FROM NEW.actor_id AND u.state='active';
  END IF;
 END CASE;
 RETURN NEW;
END $$;
CREATE TRIGGER audit_notifications AFTER INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION catalog_audit_notifications();
CREATE FUNCTION catalog_publish_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 -- Same payload is deliberately coalesced by PostgreSQL within a transaction.
 PERFORM pg_notify('catalogdb_events',json_build_object('schema',TG_TABLE_SCHEMA,'type',TG_ARGV[0])::text);
 RETURN NULL;
END $$;
DO $$ DECLARE tbl text;BEGIN
 FOREACH tbl IN ARRAY ARRAY['servers','instances','databases','backup_policies','object_attachments','lookup_values'] LOOP
  EXECUTE format('CREATE TRIGGER catalog_live AFTER INSERT OR UPDATE OR DELETE ON %I FOR EACH STATEMENT EXECUTE FUNCTION catalog_publish_change(''catalog_changed'')',tbl);
 END LOOP;
END $$;
CREATE TRIGGER notifications_live AFTER INSERT OR UPDATE OR DELETE ON notifications FOR EACH STATEMENT EXECUTE FUNCTION catalog_publish_change('notifications_changed');
CREATE TRIGGER users_live AFTER UPDATE ON users FOR EACH STATEMENT EXECUTE FUNCTION catalog_publish_change('users_changed');
DO $$ DECLARE f record;BEGIN
 FOR f IN SELECT p.oid::regprocedure signature FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname=current_schema() AND p.proname IN('catalog_audit_notifications')
 LOOP EXECUTE format('ALTER FUNCTION %s SET search_path TO %I,pg_catalog,pg_temp',f.signature,current_schema());END LOOP;
END $$;
