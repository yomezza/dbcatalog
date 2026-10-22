export function PageNavigation({
  page,
  total,
  busy,
  onPage,
}: {
  page: number;
  total: number;
  busy: boolean;
  onPage: (p: number) => void;
}) {
  return (
    <div className="pagination">
      <button disabled={busy || page === 0} onClick={() => onPage(page - 1)}>
        ←
      </button>
      <span>
        {page + 1} / {Math.max(1, Math.ceil(total / 25))} · Записей: {total}
      </span>
      <button
        disabled={busy || (page + 1) * 25 >= total}
        onClick={() => onPage(page + 1)}
      >
        →
      </button>
    </div>
  );
}
