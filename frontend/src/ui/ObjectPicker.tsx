import { useEffect, useState } from "react";
import { api, type Kind, type ObjectOption, type ObjectOptions } from "../api";
export function ObjectPicker({
  kind,
  value,
  label,
  disabled,
  onChange,
}: {
  kind: Kind;
  value: string;
  label: string;
  disabled?: boolean;
  onChange: (option: ObjectOption) => void;
}) {
  const [query, setQuery] = useState(""),
    [page, setPage] = useState(0),
    [data, setData] = useState<ObjectOptions | null>(null),
    [chosen, setChosen] = useState<ObjectOption | null>(null),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(false);
  useEffect(() => {
    let live = true;
    setLoading(true);
    const timer = setTimeout(() => {
      void api()
        .FindObjects(kind, query, page)
        .then((result) => {
          if (live) {
            setData(result);
            setError("");
          }
        })
        .catch((e) => {
          if (live) setError(String(e));
        })
        .finally(() => {
          if (live) setLoading(false);
        });
    }, 200);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [kind, query, page]);
  useEffect(() => {
    if (!value) {
      setChosen(null);
      return;
    }
    let live = true;
    void api()
      .GetObjectContext(kind, value)
      .then((c) => {
        if (live) setChosen({ object: c.object, path: c.path });
      })
      .catch(() => {});
    return () => {
      live = false;
    };
  }, [kind, value]);
  const items = [
    ...(chosen && chosen.object.id === value ? [chosen] : []),
    ...(data?.items ?? []).filter((o) => o.object.id !== value),
  ];
  return (
    <div className="object-picker">
      <input
        aria-label={"Поиск: " + label}
        placeholder="Поиск по всему каталогу"
        disabled={disabled}
        value={query}
        onChange={(e) => {
          setQuery(e.target.value);
          setPage(0);
        }}
      />
      <select
        aria-label={label}
        disabled={disabled || loading}
        required
        value={value}
        onChange={(e) => {
          const item = items.find((o) => o.object.id === e.target.value);
          if (item) {
            setChosen(item);
            onChange(item);
          }
        }}
      >
        <option value="">{loading ? "Поиск…" : "Выберите объект"}</option>
        {items.map((o) => (
          <option key={o.object.id} value={o.object.id}>
            {o.path}
          </option>
        ))}
      </select>
      {error && <small role="alert">{error}</small>}
      {data && (
        <div className="picker-pages">
          <button
            type="button"
            disabled={disabled || loading || !data.page}
            onClick={() => setPage((p) => p - 1)}
          >
            ←
          </button>
          <small>
            {data.page + 1} / {Math.max(1, Math.ceil(data.total / 25))} ·{" "}
            {data.total}
          </small>
          <button
            type="button"
            disabled={disabled || loading || (data.page + 1) * 25 >= data.total}
            onClick={() => setPage((p) => p + 1)}
          >
            →
          </button>
        </div>
      )}
    </div>
  );
}
