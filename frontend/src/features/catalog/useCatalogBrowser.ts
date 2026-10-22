import { useEffect, useMemo, useRef, useState } from "react";
import {
  api,
  type CatalogPage,
  type ObjectContext,
  type ObjectRef,
} from "../../api";
import { emptyCriteria } from "../../catalog";
const emptyPage = (): CatalogPage => ({
  items: [],
  total: 0,
  page: 0,
  pageSize: 10,
  counts: { servers: 0, instances: 0, databases: 0 },
  children: {},
});
export interface BranchPage {
  kind: "server" | "instance" | "oracle";
  id: string;
  page: number;
  loading: boolean;
  data: CatalogPage | null;
  error: string;
}
export function useCatalogBrowser(seed: CatalogPage | null, revision: number) {
  const [query, setQuery] = useState(""),
    [criteria, setCriteriaState] = useState(emptyCriteria),
    [page, setPageState] = useState(0);
  const [expanded, setExpanded] = useState<Set<string>>(new Set()),
    [collapsed, setCollapsed] = useState<Set<string>>(new Set()),
    [autoExpand, setAutoExpand] = useState(false);
  const [checked, setChecked] = useState<Map<string, ObjectRef>>(new Map()),
    [bulkOpen, setBulkOpen] = useState(false);
  const [root, setRoot] = useState<CatalogPage>(emptyPage),
    [branches, setBranches] = useState<Record<string, BranchPage>>({}),
    [loading, setLoading] = useState(false),
    [error, setError] = useState(""),
    [hiddenCount, setHiddenCount] = useState(0),
    [focusRevision, setFocusRevision] = useState(0);
  const generation = useRef(0),
    cache = useRef<Record<string, BranchPage>>({}),
    desiredPages = useRef<Record<string, number>>({}),
    pendingReveal = useRef<ObjectContext | null>(null);

  const scope = useMemo(() => ({ ...criteria, query }), [criteria, query]);
  const scopeKey = JSON.stringify(scope);
  const noCriteria =
    !query &&
    !criteria.dbms &&
    !criteria.status &&
    !criteria.environment &&
    !criteria.responsible &&
    (!criteria.policy || criteria.policy === "any");
  function key(kind: string, id: string) {
    return kind === "oracle" ? "oracle:" + id : id;
  }
  async function ensureBranch(
    kind: "server" | "instance" | "oracle",
    id: string,
    requestedPage?: number,
    retry = false,
  ) {
    const branchKey = key(kind, id),
      number = requestedPage ?? desiredPages.current[branchKey] ?? 0,
      old = cache.current[branchKey];
    if (
      !retry &&
      old?.page === number &&
      (old.loading || old.data || old.error)
    )
      return;
    desiredPages.current[branchKey] = number;
    const epoch = generation.current;
    cache.current = {
      ...cache.current,
      [branchKey]: {
        kind,
        id,
        page: number,
        loading: true,
        data: null,
        error: "",
      },
    };
    setBranches(cache.current);
    try {
      const data = await api().BrowseCatalog({
        criteria: scope,
        parentKind: kind,
        parentId: id,
        page: number,
      });
      if (epoch !== generation.current) return;
      cache.current = {
        ...cache.current,
        [branchKey]: {
          kind,
          id,
          page: data.page,
          loading: false,
          data,
          error: "",
        },
      };
      setBranches(cache.current);
    } catch (e) {
      if (epoch !== generation.current) return;
      cache.current = {
        ...cache.current,
        [branchKey]: {
          kind,
          id,
          page: number,
          loading: false,
          data: null,
          error: String(e),
        },
      };
      setBranches(cache.current);
    }
  }
  const ensureRef = useRef(ensureBranch);
  ensureRef.current = ensureBranch;
  useEffect(() => {
    const epoch = ++generation.current;
    cache.current = {};
    setBranches({});
    setLoading(true);
    setError("");
    if (!seed) {
      setRoot(emptyPage());
      setLoading(false);
      return;
    }
    const apply = (data: CatalogPage) => {
      if (epoch !== generation.current) return;
      setRoot(data);
      setLoading(false);
      const target = pendingReveal.current;
      if (target) {
        pendingReveal.current = null;
        const server = target.catalog.servers[0];
        if (server)
          void ensureRef.current(
            "server",
            server.id,
            target.branchPages[server.id] ?? 0,
          );
        for (const [id, p] of Object.entries(target.branchPages)) {
          if (id.startsWith("oracle:"))
            void ensureRef.current("oracle", id.slice(7), p);
          else if (id !== server?.id) void ensureRef.current("instance", id, p);
        }
      }
    };
    const timer = setTimeout(
      () => {
        if (seed && noCriteria && page === 0) {
          apply(seed);
          return;
        }
        void api()
          .BrowseCatalog({
            criteria: scope,
            parentKind: "",
            parentId: "",
            page,
          })
          .then(apply)
          .catch((e) => {
            if (epoch === generation.current) {
              setError(String(e));
              setLoading(false);
            }
          });
      },
      query ? 250 : 0,
    );
    return () => {
      clearTimeout(timer);
      if (generation.current === epoch) generation.current++;
    };
    // The scope key represents all query fields; selected objects never invalidate pages.
  }, [scopeKey, page, revision, focusRevision, seed]);
  useEffect(() => {
    let live = true;
    if (!checked.size) {
      setHiddenCount(0);
      return;
    }
    const timer = setTimeout(() => {
      void api()
        .CountSelectionOutside(scope, [...checked.keys()])
        .then((n) => {
          if (live) setHiddenCount(n);
        })
        .catch((e) => {
          if (live) setError(String(e));
        });
    }, 200);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [scopeKey, checked]);
  function setCriteria(value: typeof criteria) {
    desiredPages.current = {};
    setCriteriaState(value);
    setPageState(0);
  }
  function setPage(value: number) {
    desiredPages.current = {};
    setPageState(value);
  }
  function changeQuery(value: string) {
    desiredPages.current = {};
    setQuery(value);
    setPageState(0);
    setExpanded(new Set());
    setCollapsed(new Set());
    setAutoExpand(!!value);
  }
  function isExpanded(id: string) {
    return (
      expanded.has(id) ||
      (autoExpand &&
        (!id.startsWith("oracle:") || !!query) &&
        !collapsed.has(id))
    );
  }
  function toggleBranch(id: string) {
    const open = isExpanded(id);
    setExpanded((old) => {
      const next = new Set(old);
      if (open) next.delete(id);
      else next.add(id);
      return next;
    });
    setCollapsed((old) => {
      const next = new Set(old);
      if (open) next.add(id);
      else next.delete(id);
      return next;
    });
  }
  function expandAll() {
    setExpanded(
      (old) =>
        new Set([
          ...old,
          ...Object.entries(root.children)
            .filter(([, c]) => c.oracle > 0)
            .map(([id]) => "oracle:" + id),
        ]),
    );
    setAutoExpand(true);
    setCollapsed(new Set());
  }
  function collapseAll() {
    setAutoExpand(false);
    setExpanded(new Set());
    setCollapsed(new Set());
  }
  function reveal(context: ObjectContext) {
    pendingReveal.current = context;
    desiredPages.current = context.branchPages;
    setQuery("");
    setCriteriaState(emptyCriteria());
    setPageState(context.serverPage);
    setAutoExpand(false);
    setCollapsed(new Set());
    const server = context.catalog.servers[0],
      instance = context.catalog.instances[0];
    setExpanded(
      new Set([
        ...(server ? [server.id] : []),
        ...(context.object.kind === "database" && instance
          ? [instance.id]
          : []),
        ...(context.object.dbms === "Oracle Database"
          ? ["oracle:" + context.object.parentId]
          : []),
      ]),
    );
    setFocusRevision((v) => v + 1);
  }
  function clearSelection() {
    setChecked(new Map());
    setBulkOpen(false);
  }
  function reset() {
    setQuery("");
    setCriteriaState(emptyCriteria());
    setPageState(0);
    collapseAll();
    clearSelection();
    pendingReveal.current = null;
    desiredPages.current = {};
  }
  return {
    query,
    setQuery: changeQuery,
    changeQuery,
    criteria,
    setCriteria,
    page,
    setPage,
    expanded,
    isExpanded,
    toggleBranch,
    expandAll,
    collapseAll,
    checked,
    setChecked,
    bulkOpen,
    setBulkOpen,
    root,
    branches,
    ensureBranch,
    loading,
    error,
    hiddenCount,
    scope,
    reveal,
    clearSelection,
    reset,
    focusRevision,
    loadEpoch: generation.current,
    currentPage: root.page,
    lastPage: Math.max(0, Math.ceil(root.total / root.pageSize) - 1),
  };
}
export type CatalogBrowser = ReturnType<typeof useCatalogBrowser>;
