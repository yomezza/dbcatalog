export interface Diagnostics {
 generatedAt: string; appVersion: string; goVersion: string; wailsVersion: string;
 platform: string; revision: string; modified: boolean; configState: string;
 credentialBackend: string; credentialState: string; credentialCheckedAt: string;
 databaseState: string; postgresqlVersion: string; readOnly: boolean; tls: boolean;
 poolConnections: number; poolMaximum: number; migrationState: string;
 migrations: {version: number; state: string}[];
 listener: {state: string; connectedAt: string; lastEventAt: string};
}
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
  values: string[];
}
export interface ImportPreview {
  withPolicies: boolean;
  policies: ImportPolicyCandidate[];
  originalTotal: number;
  excluded: number;
  excludedRows: ImportRowPreview[];
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
  policiesCreated: number;
  policiesUpdated: number;
  excluded: number;
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
export interface DownloadEntry {
  id: string;
  name: string;
  category: string;
  createdAt: string;
  size: number;
  sha256: string;
  available: boolean;
}
export interface Notification {
  id: string;
  category: string;
  title: string;
  body: string;
  entity: string;
  objectId: string;
  createdAt: string;
  readAt: string | null;
}
export interface NotificationPage {
  items: Notification[];
  unread: number;
  total: number;
  page: number;
}
export interface Distribution {
  name: string;
  instances: number;
  databases: number;
}
export interface Statistics {
  servers: number;
  uniqueServers: number;
  serverAddresses: number;
  instances: number;
  databases: number;
  policies: number;
  enabledPolicies: number;
  protectedDatabases: number;
  unprotectedDatabases: number;
  disabledDatabases: number;
  dbms: Distribution[];
  statuses: Distribution[];
  environments: Distribution[];
  generatedAt: string;
}
export type RecordKind = Kind | "policy";
export interface RecordTarget {
  kind: RecordKind;
  id: string;
  name: string;
}
export type RecordSnapshot = CatalogObject | Policy;
export interface RecordHistoryEntry {
  id: number;
  action: string;
  actor: string;
  createdAt: string;
  before: RecordSnapshot | null;
  after: RecordSnapshot | null;
}
export interface HistoryPage {
  items: RecordHistoryEntry[];
  total: number;
  page: number;
}
export interface RecordVersion {
  version: number;
  action: string;
  actor: string;
  createdAt: string;
}
export interface VersionPage {
  items: RecordVersion[];
  total: number;
  page: number;
  currentVersion: number;
  exists: boolean;
}
interface Backend {
  BrowseCatalog(r: CatalogBrowseRequest): Promise<CatalogPage>;
  GetCatalogWorkspace(): Promise<CatalogWorkspace>;
  GetObjectContext(kind: Kind, id: string): Promise<ObjectContext>;
  FindObjects(kind: Kind, query: string, page: number): Promise<ObjectOptions>;
  SelectCatalogResult(criteria: CatalogCriteria): Promise<ObjectRef[]>;
  CountSelectionOutside(
    criteria: CatalogCriteria,
    ids: string[],
  ): Promise<number>;
  ExportCatalogResult(criteria: CatalogCriteria): Promise<DownloadEntry>;
  PreviewMove(r: MoveRequest): Promise<MovePreview>;
  ApplyMove(r: MoveRequest, fingerprint: string): Promise<CatalogObject>;
  GetMovements(kind: Kind, id: string, page: number): Promise<MovementPage>;
  ListAttachments(
    kind: Kind,
    id: string,
    page: number,
  ): Promise<AttachmentPage>;
  UploadAttachment(incoming: AttachmentUpload): Promise<Attachment>;
  SaveInstruction(
    kind: Kind,
    id: string,
    attachmentId: string,
    version: number,
    instruction: Instruction,
  ): Promise<Attachment>;
  InstructionTemplate(
    kind: Kind,
    id: string,
    template: string,
  ): Promise<Instruction>;
  DeleteAttachment(id: string, version: number): Promise<void>;
  DownloadAttachment(id: string): Promise<DownloadEntry>;
  GetPolicyPage(query: string, page: number): Promise<PolicyPage>;

  GetWorkspace(): Promise<Workspace>;
  PreviewExcelPolicies(
    token: string,
    c: PolicyImportConfig,
  ): Promise<ImportPreview>;
  ListFilterPresets(): Promise<FilterPreset[]>;
  SaveFilterPreset(f: FilterPreset): Promise<FilterPreset>;
  DeleteFilterPreset(id: string, version: number): Promise<void>;
  PreviewBulkEdit(refs: ObjectRef[], patch: BulkPatch): Promise<BulkPreview>;
  ApplyBulkEdit(
    refs: ObjectRef[],
    patch: BulkPatch,
    fingerprint: string,
  ): Promise<number>;
  GetQualityReport(code: string, page: number): Promise<QualityPage>;
  ListDeleted(page: number): Promise<DeletedPage>;
  PreviewDeletedRestore(
    kind: string,
    id: string,
    branch: boolean,
  ): Promise<RestorePreview>;
  RestoreDeleted(
    kind: string,
    id: string,
    branch: boolean,
    fingerprint: string,
  ): Promise<number>;
  ExportSelection(ids: string[]): Promise<DownloadEntry>;

  SetImportRowsExcluded(
    token: string,
    rows: number[],
    exclude: boolean,
  ): Promise<ImportPreview>;
  GetHistoryPage(
    kind: RecordKind,
    id: string,
    page: number,
  ): Promise<HistoryPage>;
  GetVersions(kind: RecordKind, id: string, page: number): Promise<VersionPage>;
  GetRecordVersion(
    kind: RecordKind,
    id: string,
    version: number,
  ): Promise<RecordSnapshot>;
  RestoreVersion(
    kind: RecordKind,
    id: string,
    currentVersion: number,
    targetVersion: number,
  ): Promise<void>;
  RestoreObject(
    kind: Kind,
    id: string,
    version: number,
    historyId: number,
    side: "before" | "after",
  ): Promise<CatalogObject>;
  GetStatistics(): Promise<Statistics>;
  GetNotifications(
    page: number,
    unreadOnly: boolean,
  ): Promise<NotificationPage>;
  MarkNotificationsRead(ids: string[], all: boolean): Promise<void>;
  ListDownloads(): Promise<DownloadEntry[]>;
  ExportToDownloads(): Promise<DownloadEntry>;
  ExportImportPreview(token: string): Promise<DownloadEntry>;
  OpenDownload(id: string): Promise<void>;
  ShowDownload(id: string): Promise<void>;
  SaveDownloadAs(id: string): Promise<boolean>;
  ConnectToServer(kind: Kind, id: string): Promise<void>;
  RevalidateExcel(
    token: string,
    edits: { row: number; values: string[] }[],
  ): Promise<ImportPreview>;

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
  GetDiagnostics(): Promise<Diagnostics>;
  SaveDiagnosticsReport(): Promise<boolean>;
  Register(login: string, password: string, bootstrap: boolean): Promise<void>;
  Login(login: string, password: string): Promise<User>;
  LoginWithSession(
    login: string,
    password: string,
    hours: number,
  ): Promise<User>;
  ResumeSession(): Promise<Status>;
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
  SetUserPassword(id: string, password: string): Promise<void>;
  GetHistory(kind: Kind, id: string): Promise<HistoryEntry[]>;
}
declare global {
  interface Window {
    go?: { main: { App: Backend } };
    runtime?: {
      WindowMinimise(): void;
      WindowToggleMaximise(): void;
      Quit(): void;
      EventsOn?(
        name: string,
        callback: (change: { schema: string; type: string }) => void,
      ): () => void;
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

export interface CatalogCriteria {
  query: string;
  dbms: string;
  status: string;
  environment: string;
  responsible: string;
  policy: string;
}
export interface FilterPreset {
  id: string;
  name: string;
  version: number;
  criteria: CatalogCriteria;
}
export interface ObjectRef {
  id: string;
  version: number;
}
export interface BulkPatch {
  setResponsible: boolean;
  responsible: string;
  setOrganization: boolean;
  organization: string;
  setStatus: boolean;
  status: string;
  setAdministration: boolean;
  administration: string[];
}
export interface BulkPreview {
  fingerprint: string;
  selected: number;
  changed: number;
  items: { before: CatalogObject; after: CatalogObject }[];
}
export interface QualityIssue {
  code: string;
  severity: string;
  kind: Kind;
  id: string;
  name: string;
  path: string;
  detail: string;
}
export interface QualityPage {
  items: QualityIssue[];
  counts: Record<string, number>;
  total: number;
  page: number;
}
export interface DeletedRecord {
  kind: Kind | "policy";
  id: string;
  batchId: string;
  name: string;
  deletedAt: string;
  actor: string;
  snapshot: CatalogObject | Policy;
}
export interface DeletedPage {
  items: DeletedRecord[];
  total: number;
  page: number;
}
export interface RestorePreview {
  items: DeletedRecord[];
  conflicts: string[];
  fingerprint: string;
}
export interface PolicyImportOverride {
  row: number;
  skip: boolean;
  policy: Policy;
}
export interface PolicyImportConfig {
  enabled: boolean;
  targetKind: string;
  defaultState: string;
  retentionUnit: string;
  defaults: Policy;
  overrides: PolicyImportOverride[];
}
export interface ImportPolicyCandidate {
  row: number;
  target: string;
  action: string;
  policy: Policy;
  before: Policy | null;
  errors: string[];
  skipped: boolean;
}

export interface Workspace {
  catalog: Catalog;
  lookups: Lookups;
  favorites: CatalogObject[];
  policies: Policy[];
}

export interface CatalogCounts {
  servers: number;
  instances: number;
  databases: number;
}
export interface ChildCounts {
  instances: number;
  databases: number;
  oracle: number;
}
export interface CatalogPage {
  items: CatalogObject[];
  total: number;
  page: number;
  pageSize: number;
  counts: CatalogCounts;
  children: Record<string, ChildCounts>;
}
export interface CatalogBrowseRequest {
  criteria: CatalogCriteria;
  parentKind: "" | "server" | "instance" | "oracle";
  parentId: string;
  page: number;
}
export interface CatalogWorkspace {
  root: CatalogPage;
  lookups: Lookups;
  favorites: CatalogObject[];
}
export interface ObjectContext {
  object: CatalogObject;
  catalog: Catalog;
  policies: Policy[];
  serverPage: number;
  branchPages: Record<string, number>;
  path: string;
}
export interface ObjectOption {
  object: CatalogObject;
  path: string;
}
export interface ObjectOptions {
  items: ObjectOption[];
  total: number;
  page: number;
}
export interface Instruction {
  title: string;
  template: string;
  purpose: string;
  prerequisites: string;
  steps: string;
  verification: string;
  rollback: string;
  notes: string;
}
export interface Attachment {
  id: string;
  kind: Kind;
  objectId: string;
  name: string;
  description: string;
  fileName: string;
  mime: string;
  size: number;
  sha256: string;
  version: number;
  actor: string;
  updatedAt: string;
  instruction: Instruction | null;
}
export interface AttachmentPage {
  items: Attachment[];
  total: number;
  page: number;
}
export interface AttachmentUpload {
  kind: Kind;
  objectId: string;
  name: string;
  description: string;
  fileName: string;
  data: string;
}
export interface MoveRequest {
  kind: Kind;
  id: string;
  version: number;
  parentId: string;
  parentVersion: number;
  reason: string;
}
export interface MovePreview {
  fromDBMS: string;
  toDBMS: string;
  object: CatalogObject;
  destination: CatalogObject;
  fromPath: string;
  toPath: string;
  fromPolicy: Policy | null;
  toPolicy: Policy | null;
  databases: number;
  attachments: number;
  fingerprint: string;
  conflicts: string[];
}
export interface Movement {
  id: number;
  fromPath: string;
  toPath: string;
  reason: string;
  version: number;
  actor: string;
  createdAt: string;
  fromPolicy: Policy | null;
  toPolicy: Policy | null;
}
export interface MovementPage {
  items: Movement[];
  total: number;
  page: number;
}
export interface PolicyPage {
  items: Policy[];
  paths: Record<string, string>;
  total: number;
  page: number;
}
