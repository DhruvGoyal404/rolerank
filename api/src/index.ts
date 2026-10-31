import { Redis } from "iovalkey";
import pg from "pg";
import { githubProvider, googleProvider } from "./auth/providers.js";
import { loadConfig } from "./config.js";
import { createApp } from "./http/app.js";
import { createLog } from "./log.js";

const log = createLog();

try {
  const config = loadConfig(process.env);
  const pool = new pg.Pool({
    connectionString: config.appDatabaseUrl,
    statement_timeout: 10_000,
    max: 10,
  });
  const valkey = new Redis(config.valkeyUrl, { lazyConnect: false, enableOfflineQueue: true });
  const app = createApp({
    config,
    pool,
    valkey,
    log,
    providers: { google: googleProvider(config.google), github: githubProvider(config.github) },
  });
  const server = app.listen(config.port, "0.0.0.0", () => {
    log.info({ event: "listening", status: config.port });
  });
  const stop = (): void => {
    server.close(() => {
      valkey.disconnect();
      void pool.end().then(() => process.exit(0));
    });
  };
  process.on("SIGTERM", stop);
  process.on("SIGINT", stop);
} catch (error) {
  // Configuration messages name fields and rules, never values.
  console.error((error as Error).message);
  process.exit(1);
}
