import { useMemo, useState } from "react";
import type { Catalog, CatalogObject, ObjectRef, Policy } from "../../api";
import {
  filterCatalog,
  filterCatalogCriteria,
  branchKeys,
  emptyCriteria,
} from "../../catalog";
import {
  indexCatalog,
  objectLocation,
  hiddenSelectionCount,
} from "../../catalog/indexes";
export function useCatalogBrowser(catalog: Catalog, policies: Policy[]) {
  const [query, setQuery] = useState(""),
    [criteria, setCriteria] = useState(emptyCriteria),
    [page, setPage] = useState(0);
  const [expanded, setExpanded] = useState<Set<string>>(new Set()),
    [checked, setChecked] = useState<Map<string, ObjectRef>>(new Map()),
    [bulkOpen, setBulkOpen] = useState(false);
  const index = useMemo(() => indexCatalog(catalog), [catalog]);
  const filtered = useMemo(
    () => filterCatalogCriteria(catalog, query, criteria, policies, index),
    [catalog, index, query, criteria, policies],
  );
  const visibleIds = useMemo(
    () => new Set(filtered.databases.map((d) => d.id)),
    [filtered.databases],
  );
  const hiddenCount = useMemo(
    () => hiddenSelectionCount(checked, visibleIds),
    [checked, visibleIds],
  );
  const lastPage = Math.max(0, Math.ceil(filtered.servers.length / 10) - 1),
    currentPage = Math.min(page, lastPage);
  function changeQuery(value: string) {
    setQuery(value);
    setPage(0);
    setExpanded(
      value
        ? new Set(branchKeys(filterCatalog(catalog, value, index)))
        : new Set(),
    );
  }
  function toggleBranch(id: string) {
    setExpanded((old) => {
      const next = new Set(old);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  }
  function reveal(object: CatalogObject, source: Catalog = catalog) {
    const location = objectLocation(
      source === catalog ? index : indexCatalog(source),
      object,
    );
    setQuery("");
    setCriteria(emptyCriteria());
    setPage(location.page);
    setExpanded((old) => new Set([...old, ...location.branches]));
  }
  function clearSelection() {
    setChecked(new Map());
    setBulkOpen(false);
  }
  function reset() {
    setQuery("");
    setCriteria(emptyCriteria());
    setPage(0);
    setExpanded(new Set());
    clearSelection();
  }
  return {
    query,
    setQuery,
    changeQuery,
    criteria,
    setCriteria,
    page,
    setPage,
    expanded,
    setExpanded,
    toggleBranch,
    checked,
    setChecked,
    bulkOpen,
    setBulkOpen,
    index,
    filtered,
    currentPage,
    lastPage,
    hiddenCount,
    reveal,
    clearSelection,
    reset,
  };
}
export type CatalogBrowser = ReturnType<typeof useCatalogBrowser>;
