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
export interface CatalogObject {
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
interface Backend {
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
  DeleteObject(kind: Kind, id: string, version: number): Promise<void>;
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
    status: "Активен",
    description: "",
    dbms: "PostgreSQL",
    dbmsVersion: "",
    port: null,
    purpose: "",
    version: 0,
  };
}
