import type { Catalog, RecordKind, RecordSnapshot } from "./api";
import { recordFields, recordValue, changedFields } from "./recordFields";
export function SnapshotTable({
  before,
  after,
  kind,
  catalog,
  onlyChanges = false,
  labels = ["Было", "Стало"],
}: {
  before: RecordSnapshot | null;
  after: RecordSnapshot | null;
  kind: RecordKind;
  catalog: Catalog;
  onlyChanges?: boolean;
  labels?: [string, string];
}) {
  const fields = onlyChanges
    ? changedFields(before, after, kind)
    : recordFields(kind);
  return fields.length ? (
    <div className="snapshot-table">
      <table>
        <thead>
          <tr>
            <th scope="col">Поле</th>
            <th scope="col">{labels[0]}</th>
            <th scope="col">{labels[1]}</th>
          </tr>
        </thead>
        <tbody>
          {fields.map(([key, label]) => (
            <tr key={key}>
              <td>{label}</td>
              <td>{recordValue(before, kind, key, catalog)}</td>
              <td>{recordValue(after, kind, key, catalog)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  ) : (
    <p className="muted">Значения полей совпадают.</p>
  );
}
