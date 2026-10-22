import type {
  Catalog,
  CatalogObject,
  RecordKind,
  RecordSnapshot,
  Policy,
} from "./api";
import { fieldLabels, objectValue } from "./objectFields";
import { objectPath } from "./objectPath";
export const actionLabels: Record<string, string> = {
  create: "Создание",
  update: "Изменение",
  delete: "Удаление",
  restore: "Восстановление",
  baseline: "Исходное состояние",
};
const policyLabels: Record<string, string> = {
  name: "Название",
  kind: "Уровень назначения",
  objectId: "Объект назначения",
  enabled: "Статус",
  backupType: "Тип РК",
  scheduler: "Планировщик",
  schedule: "Расписание",
  storage: "Место хранения",
  retentionDays: "Срок хранения, дней",
  description: "Примечания",
};
export function recordFields(kind: RecordKind) {
  return Object.entries(kind === "policy" ? policyLabels : fieldLabels).filter(
    ([key]) => key !== "version",
  );
}
export function recordValue(
  snapshot: RecordSnapshot | null,
  kind: RecordKind,
  key: string,
  catalog: Catalog,
): string {
  if (!snapshot) return "—";
  if (kind === "policy") {
    const p = snapshot as Policy;
    if (key === "enabled") return p.enabled ? "Включена" : "Отключена";
    if (key === "kind")
      return p.kind === "instance" ? "Экземпляр СУБД" : "База данных";
    if (key === "objectId") {
      const o = (
        p.kind === "instance" ? catalog.instances : catalog.databases
      ).find((o) => o.id === p.objectId);
      return o ? objectPath(o, catalog) : "Объект отсутствует";
    }
    return String(p[key as keyof Policy] ?? "—");
  }
  const o = snapshot as CatalogObject;
  if (key === "parentId") {
    if (o.kind === "server") return "—";
    const parents =
      o.kind === "instance" || o.dbms === "Oracle Database"
        ? catalog.servers
        : catalog.instances;
    const p = parents.find((p) => p.id === o.parentId);
    return p ? objectPath(p, catalog) : "Родитель отсутствует";
  }
  return objectValue(o, key) || "—";
}
export function changedFields(
  before: RecordSnapshot | null,
  after: RecordSnapshot | null,
  kind: RecordKind,
) {
  return recordFields(kind).filter(
    ([key]) =>
      JSON.stringify(rawValue(before, key)) !==
      JSON.stringify(rawValue(after, key)),
  );
}
function rawValue(o: RecordSnapshot | null, key: string): unknown {
  if (!o) return null;
  if (key.startsWith("attributes."))
    return (o as CatalogObject).attributes?.[
      key.slice(11) as keyof CatalogObject["attributes"]
    ];
  return (o as unknown as Record<string, unknown>)[key];
}
