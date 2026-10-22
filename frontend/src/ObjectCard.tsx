import type { Catalog, CatalogObject, Policy } from "./api";
import { objectPath } from "./objectPath";
import { objectValue, fieldLabels } from "./objectFields";
import { ValueBadge } from "./ValueBadge";
import { BackupCard } from "./PoliciesPanel";
const labels = {
  server: "Сервер",
  instance: "Экземпляр СУБД",
  database: "База данных",
};
function Detail({
  object,
  field,
  badge = false,
}: {
  object: CatalogObject;
  field: string;
  badge?: boolean;
}) {
  const value = objectValue(object, field);
  if (!value) return null;
  const values =
    field === "attributes.administration"
      ? (object.attributes?.administration ?? [])
      : [value];
  return (
    <div className="card-field">
      <dt>{fieldLabels[field]}</dt>
      <dd>
        {badge ? (
          <div className="badge-list">
            {values.map((v) => (
              <ValueBadge key={v} value={v} category={field.split(".").pop()} />
            ))}
          </div>
        ) : (
          value
        )}
      </dd>
    </div>
  );
}
interface Props {
  object: CatalogObject | null;
  catalog: Catalog;
  policies: Policy[];
  busy: boolean;
  canEdit: boolean;
  canDelete: boolean;
  favorite: boolean;
  onEdit: () => void;
  onAdd: () => void;
  onAddOracle: () => void;
  onFavorite: () => void;
  onHistory: () => void;
  onVersions: () => void;
  onDelete: () => void;
  onPolicy: (o: CatalogObject) => void;
}
export function ObjectCard(p: Props) {
  const o = p.object;
  if (!o)
    return (
      <section className="detail-panel">
        <div className="empty detail-empty">
          <span>◈</span>
          <h2>Выберите объект</h2>
          <p>Откройте сервер, экземпляр или базу в дереве слева.</p>
        </div>
      </section>
    );
  const placement =
    o.kind === "server"
      ? [
          "address",
          "attributes.location",
          "attributes.connectionMethod",
          "attributes.domain",
          "os",
          "attributes.osVersion",
          "attributes.osFamily",
        ]
      : o.kind === "instance"
        ? ["dbmsVersion", "port"]
        : [
            ...(o.dbms === "Oracle Database" ? ["dbmsVersion", "port"] : []),
            "attributes.informationSystem",
            "purpose",
            "attributes.personalData",
          ];
  const support = [
    "attributes.serviceOrganization",
    "attributes.supervisor",
    "attributes.responsible",
    "attributes.administration",
  ];
  const hasSupport = support.some((k) => objectValue(o, k));
  return (
    <section className="detail-panel object-card">
      <header className="object-card-header">
        <div className={"object-card-icon " + o.kind}>
          {o.kind === "server" ? "▤" : o.kind === "instance" ? "◈" : "▱"}
        </div>
        <div className="object-card-title">
          <div className="eyebrow">{labels[o.kind]}</div>
          <h2>{o.name}</h2>
          <p className="object-path">{objectPath(o, p.catalog)}</p>
        </div>
        <button
          className={"favorite-icon " + (p.favorite ? "is-favorite" : "")}
          disabled={p.busy}
          aria-label={
            p.favorite ? "Убрать из избранного" : "Добавить в избранное"
          }
          title={p.favorite ? "Убрать из избранного" : "Добавить в избранное"}
          onClick={p.onFavorite}
        >
          {p.favorite ? "★" : "☆"}
        </button>
      </header>
      <div className="object-flags">
        {o.environment && <ValueBadge value={o.environment} />}
        <ValueBadge value={o.status} />
        {(o.kind === "instance" || o.dbms === "Oracle Database") && o.dbms && (
          <ValueBadge value={o.dbms} />
        )}
      </div>
      {placement.some((k) => objectValue(o, k)) && (
        <section className="card-section">
          <h3>
            {o.kind === "server"
              ? "Размещение и доступ"
              : o.kind === "instance"
                ? "Параметры экземпляра"
                : "Сведения о базе данных"}
          </h3>
          <dl className="card-fields">
            {placement.map((k) => (
              <Detail
                key={k}
                object={o}
                field={k}
                badge={
                  k.startsWith("attributes.") &&
                  ![
                    "attributes.osVersion",
                    "attributes.osFamily",
                    ...(o.dbms === "Oracle Database"
                      ? ["dbmsVersion", "port"]
                      : []),
                    "attributes.informationSystem",
                  ].includes(k)
                }
              />
            ))}
          </dl>
        </section>
      )}
      {hasSupport && (
        <section className="card-section">
          <h3>Сопровождение</h3>
          <dl className="card-fields">
            {support.map((k) => (
              <Detail
                key={k}
                object={o}
                field={k}
                badge={
                  k === "attributes.administration" ||
                  k === "attributes.serviceOrganization"
                }
              />
            ))}
          </dl>
        </section>
      )}
      {o.description && (
        <section className="card-section">
          <h3>Описание</h3>
          <p className="object-description">{o.description}</p>
        </section>
      )}
      {o.kind !== "server" && (
        <BackupCard
          object={o}
          policies={p.policies}
          canEdit={p.canEdit}
          onEdit={p.onPolicy}
        />
      )}
      <div className="detail-actions">
        {p.canEdit && (
          <>
            <button disabled={p.busy} onClick={p.onEdit}>
              Изменить
            </button>
            {o.kind === "server" && (
              <button disabled={p.busy} onClick={p.onAddOracle}>
                + База Oracle
              </button>
            )}
            {o.kind !== "database" && (
              <button disabled={p.busy} onClick={p.onAdd}>
                + {o.kind === "server" ? "Экземпляр СУБД" : "База данных"}
              </button>
            )}
          </>
        )}
        <button disabled={p.busy} onClick={p.onHistory}>
          История изменений
        </button>
        {p.canDelete && (
          <button disabled={p.busy} onClick={p.onVersions}>
            Версии записи
          </button>
        )}
        {p.canDelete && (
          <button className="danger" disabled={p.busy} onClick={p.onDelete}>
            Удалить
          </button>
        )}
      </div>
    </section>
  );
}
