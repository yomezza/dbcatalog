import type { CatalogObject } from "../../api";
import { isAndroid } from "../../android";
interface Props {
  object: CatalogObject;
  selected: boolean;
  checked: boolean;
  busy: boolean;
  onSelect: (object: CatalogObject) => void;
  onCheck: (object: CatalogObject, value: boolean) => void;
  onToggle: (id: string) => void;
  onContext: (object: CatalogObject, x: number, y: number) => void;
}
export function CatalogNode({
  object: o,
  selected,
  checked,
  busy,
  onSelect,
  onCheck,
  onToggle,
  onContext,
}: Props) {
  return (
    <>
      {o.kind === "database" && (
        <input
          type="checkbox"
          className="tree-check"
          aria-label={"Выбрать БД " + o.name}
          checked={checked}
          disabled={busy}
          onChange={(e) => onCheck(o, e.target.checked)}
        />
      )}
      <button
        className={"tree-node " + (selected ? "selected" : "")}
        onClick={() => onSelect(o)}
        onDoubleClick={() => {
          if (o.kind !== "database") onToggle(o.id);
        }}
        onContextMenu={(e) => {
          e.preventDefault();
          onContext(o, e.clientX, e.clientY);
        }}
        onKeyDown={(e) => {
          if (e.key === "ContextMenu" || (e.shiftKey && e.key === "F10")) {
            e.preventDefault();
            const rect = e.currentTarget.getBoundingClientRect();
            onContext(o, rect.left + 20, rect.bottom);
          }
        }}
      >
        <span className={"node-icon " + o.kind}>
          {o.kind === "server" ? "▤" : o.kind === "instance" ? "◈" : "▱"}
        </span>
        <span>
          {o.name}
          <small>
            {o.kind === "server"
              ? o.address
              : o.kind === "instance"
                ? o.dbms
                : o.purpose}
          </small>
        </span>
      </button>
      {isAndroid() && (
        <button
          type="button"
          className="touch-object-actions"
          aria-label={`Действия: ${o.name}`}
          aria-haspopup="menu"
          disabled={busy}
          onClick={(event) => {
            const rect = event.currentTarget.getBoundingClientRect();
            onContext(o, rect.right, rect.bottom);
          }}
        >
          ⋮
        </button>
      )}
    </>
  );
}
