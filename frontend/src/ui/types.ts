export type AsyncTaskRunner = (task: () => Promise<void>) => Promise<void>;
