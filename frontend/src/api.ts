export interface Connection {
  host: string;
  port: number;
  database: string;
  username: string;
  schema: string;
  sslMode: string;
  sslRootCert: string;
}
export interface User {
  id: string;
  login: string;
  role: "admin" | "editor" | "viewer";
  state: string;
}
export interface Status {
  connected: boolean;
  schema: string;
  needsAdmin: boolean;
  user: User | null;
}
export type Kind = "server" | "instance" | "database";
export interface Attributes {
  connectionMethod: string;
  domain: string;
  location: string;
  serviceOrganization: string;
  supervisor: string;
  responsible: string;
  osVersion: string;
  osFamily: string;
  administration: string[];
  personalData: string;
  informationSystem: string;
}
export type Lookups = Record<string, string[]>;
export const emptyAttributes = (): Attributes => ({
  connectionMethod: "",
  domain: "",
  location: "",
  serviceOrganization: "",
  supervisor: "",
  responsible: "",
  osVersion: "",
  osFamily: "",
  administration: [],
  personalData: "",
  informationSystem: "",
});
export interface CatalogObject {
  attributes: Attributes;
  id: string;
  kind: Kind;
  parentId: string;
  name: string;
  address: string;
  os: string;
  environment: string;
  status: string;
  description: string;
  dbms: string;
  dbmsVersion: string;
  port: number | null;
  purpose: string;
  version: number;
}
export interface Catalog {
  servers: CatalogObject[];
  instances: CatalogObject[];
  databases: CatalogObject[];
}
export interface HistoryEntry {
  id: number;
  action: string;
  actor: string;
  createdAt: string;
  before: CatalogObject | null;
  after: CatalogObject | null;
}
export interface ImportRowPreview {
  row: number;
  server: string;
  instance: string;
  database: string;
  errors: string[];
}
export interface ImportPreview {
  token: string;
  sheet: string;
  columns: number;
  total: number;
  valid: number;
  ignored: number;
  secretsIgnored: number;
  backupRows: number;
  rows: ImportRowPreview[];
}
export interface ImportResult {
  serversCreated: number;
  instancesCreated: number;
  databasesCreated: number;
  objectsUpdated: number;
  skipped: number;
}
export interface Policy {
  id: string;
  kind: "instance" | "database";
  objectId: string;
  name: string;
  enabled: boolean;
  backupType: string;
  scheduler: string;
  schedule: string;
  storage: string;
  retentionDays: number;
  description: string;
  version: number;
}
export interface PolicyHistoryEntry {
  id: number;
  action: string;
  actor: string;
  createdAt: string;
  before: Policy | null;
  after: Policy | null;
}
export function newPolicy(
  kind: "instance" | "database" = "instance",
  objectId = "",
): Policy {
  return {
    id: "",
    kind,
    objectId,
    name: "Политика РК",
    enabled: false,
    backupType: "Не указано",
    scheduler: "Не указано",
    schedule: "",
    storage: "",
    retentionDays: 0,
    description: "",
    version: 0,
  };
}
export interface DeletionPreview {
  kind: Kind | "all";
  name: string;
  servers: number;
  instances: number;
  databases: number;
  policies: number;
  fingerprint: string;
}
export interface AuditFilter {
  query: string;
  actorId: string;
  action: string;
  entity: string;
  from: string;
  to: string;
  page: number;
  pageSize: number;
}
export interface AuditEvent {
  id: string;
  actorId: string;
  actor: string;
  action: string;
  actionLabel: string;
  entity: string;
  objectId: string;
  objectName: string;
  createdAt: string;
  details: Record<string, unknown>;
}
export interface AuditPage {
  events: AuditEvent[];
  total: number;
  page: number;
  pageSize: number;
  actors: { id: string; login: string }[];
  actions: Record<string, string>;
}
interface Backend {
  ListPolicies(): Promise<Policy[]>;
  SavePolicy(p: Policy): Promise<Policy>;
  DeletePolicy(id: string, version: number): Promise<void>;
  GetEffectivePolicy(
    id: string,
  ): Promise<{ policy: Policy | null; inherited: boolean }>;
  GetPolicyHistory(id: string): Promise<PolicyHistoryEntry[]>;
  PreviewExcel(encoded: string): Promise<ImportPreview>;
  ApplyExcel(
    token: string,
    skipInvalid: boolean,
    catalogOnly: boolean,
  ): Promise<ImportResult>;
  ExportExcel(): Promise<string>;
  GetLookups(): Promise<Lookups>;
  ListFavorites(): Promise<CatalogObject[]>;
  SetFavorite(kind: Kind, id: string, enabled: boolean): Promise<void>;
  IsSetupMode(): Promise<boolean>;
  GetConnection(): Promise<{ connection: Connection; hasPassword: boolean }>;
  Connect(
    c: Connection,
    p: string,
    saved: boolean,
    remember: boolean,
  ): Promise<Status>;
  GetStatus(): Promise<Status>;
  Register(login: string, password: string, bootstrap: boolean): Promise<void>;
  Login(login: string, password: string): Promise<User>;
  Logout(): Promise<void>;
  GetCatalog(): Promise<Catalog>;
  SaveObject(o: CatalogObject): Promise<CatalogObject>;
  DeleteObject(
    kind: Kind,
    id: string,
    version: number,
    fingerprint: string,
  ): Promise<DeletionPreview>;
  PreviewDeletion(
    kind: Kind,
    id: string,
    version: number,
  ): Promise<DeletionPreview>;
  PreviewClearCatalog(): Promise<DeletionPreview>;
  ClearCatalog(
    fingerprint: string,
    confirmation: string,
  ): Promise<DeletionPreview>;
  GetAudit(filter: AuditFilter): Promise<AuditPage>;
  ListUsers(): Promise<User[]>;
  UpdateUser(id: string, role: string, state: string): Promise<void>;
  GetHistory(kind: Kind, id: string): Promise<HistoryEntry[]>;
}
declare global {
  interface Window {
    go?: { main: { App: Backend } };
    runtime?: {
      WindowMinimise(): void;
      WindowToggleMaximise(): void;
      Quit(): void;
    };
  }
}
export function api(): Backend {
  if (!window.go)
    throw new Error(
      "Откройте приложение CatalogDB через Wails. Подключение к БД недоступно в обычном браузере.",
    );
  return window.go.main.App;
}
export const emptyCatalog: Catalog = {
  servers: [],
  instances: [],
  databases: [],
};
export function newObject(kind: Kind, parentId = ""): CatalogObject {
  return {
    id: "",
    kind,
    parentId,
    name: "",
    address: "",
    os: "",
    environment: "",
    status: "Эксплуатируется",
    attributes: emptyAttributes(),
    description: "",
    dbms: kind === "instance" ? "PostgreSQL" : "",
    dbmsVersion: "",
    port: null,
    purpose: "",
    version: 0,
  };
}
