import type { CatalogObject } from "./api";
const labels = {
  server: "Сервер",
  instance: "Экземпляр СУБД",
  database: "База данных",
};
export function FavoritesPanel({
  objects,
  busy,
  onOpen,
  onRemove,
  onContext,
}: {
  objects: CatalogObject[];
  busy: boolean;
  onOpen: (o: CatalogObject) => void;
  onRemove: (o: CatalogObject) => void;
  onContext: (e: React.MouseEvent, o: CatalogObject) => void;
}) {
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">ЛИЧНАЯ КОЛЛЕКЦИЯ</div>
          <h1>Избранное</h1>
          <p className="muted">Ваши серверы, экземпляры и базы данных</p>
        </div>
      </div>
      <section className="table-panel">
        {objects.length === 0 ? (
          <div className="empty">
            В избранном пока нет объектов. Добавьте их через ★ в карточке или
            контекстное меню.
          </div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Объект</th>
                <th>Тип</th>
                <th>Состояние</th>
                <th>Действия</th>
              </tr>
            </thead>
            <tbody>
              {objects.map((o) => (
                <tr key={o.kind + o.id} onContextMenu={(e) => onContext(e, o)}>
                  <td>
                    <button className="link" onClick={() => onOpen(o)}>
                      {o.name}
                    </button>
                  </td>
                  <td>{labels[o.kind]}</td>
                  <td>{o.status}</td>
                  <td>
                    <button disabled={busy} onClick={() => onRemove(o)}>
                      Убрать из избранного
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </>
  );
}
