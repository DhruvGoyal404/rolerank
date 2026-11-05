import { z } from "zod";

const GOOGLE_ISSUER = "https://accounts.google.com";
const GITHUB_WEB_URL = "https://github.com";
const GITHUB_API_URL = "https://api.github.com";

const schema = z.object({
  NODE_ENV: z.enum(["development", "test", "production"]),
  PORT: z.coerce.number().int().min(1).max(65535).default(8080),
  PUBLIC_ORIGIN: z.url(),
  APP_DATABASE_URL: z.string().min(1),
  VALKEY_URL: z.string().min(1),
  GOOGLE_CLIENT_ID: z.string().min(1),
  GOOGLE_CLIENT_SECRET: z.string().min(1),
  GITHUB_CLIENT_ID: z.string().min(1),
  GITHUB_CLIENT_SECRET: z.string().min(1),
  GOOGLE_ISSUER: z.url().default(GOOGLE_ISSUER),
  GITHUB_WEB_URL: z.url().default(GITHUB_WEB_URL),
  GITHUB_API_URL: z.url().default(GITHUB_API_URL),
  TEST_AUTH_SHORTCUT: z.string().optional(),
  // The key-encryption key for resume data: 32 bytes, base64. Read from SSM in production.
  ROLERANK_KEK: z.string().optional(),
  ROLERANK_KEK_VERSION: z.coerce.number().int().min(1).default(1),
});

export interface Config {
  env: "development" | "test" | "production";
  port: number;
  publicOrigin: string;
  appDatabaseUrl: string;
  valkeyUrl: string;
  google: { clientId: string; clientSecret: string; issuer: string };
  github: { clientId: string; clientSecret: string; webUrl: string; apiUrl: string };
  testAuthShortcut: boolean;
  secureCookies: boolean;
  /** Absent in development without resume features; required in production. */
  kek: { key: Buffer; version: number } | null;
}

function databaseName(url: string): string {
  return new URL(url).pathname.replace(/^\//, "");
}

/** Refuses to return a config that production must never run with. */
export function loadConfig(env: NodeJS.ProcessEnv): Config {
  const parsed = schema.safeParse(env);
  if (!parsed.success) {
    const fields = parsed.error.issues.map((issue) => issue.path.join(".")).join(", ");
    throw new Error(`invalid configuration: ${fields}`);
  }
  const c = parsed.data;

  const shortcut = c.TEST_AUTH_SHORTCUT === "1";
  if (shortcut && c.NODE_ENV !== "test") {
    throw new Error("TEST_AUTH_SHORTCUT is allowed only with NODE_ENV=test");
  }
  if (c.NODE_ENV === "test" && !databaseName(c.APP_DATABASE_URL).endsWith("_test")) {
    throw new Error("NODE_ENV=test needs a database whose name ends in _test");
  }
  const kek = c.ROLERANK_KEK ? Buffer.from(c.ROLERANK_KEK, "base64") : null;
  if (kek && kek.length !== 32) throw new Error("ROLERANK_KEK must be 32 bytes, base64");
  if (!kek && c.NODE_ENV === "production") throw new Error("ROLERANK_KEK is required in production");
  if (c.NODE_ENV === "production") {
    if (!c.PUBLIC_ORIGIN.startsWith("https://")) {
      throw new Error("PUBLIC_ORIGIN must be https in production");
    }
    if (
      c.GOOGLE_ISSUER !== GOOGLE_ISSUER ||
      c.GITHUB_WEB_URL !== GITHUB_WEB_URL ||
      c.GITHUB_API_URL !== GITHUB_API_URL
    ) {
      throw new Error("OAuth provider URLs can't be overridden in production");
    }
  }

  return {
    env: c.NODE_ENV,
    port: c.PORT,
    publicOrigin: new URL(c.PUBLIC_ORIGIN).origin,
    appDatabaseUrl: c.APP_DATABASE_URL,
    valkeyUrl: c.VALKEY_URL,
    google: {
      clientId: c.GOOGLE_CLIENT_ID,
      clientSecret: c.GOOGLE_CLIENT_SECRET,
      issuer: c.GOOGLE_ISSUER,
    },
    github: {
      clientId: c.GITHUB_CLIENT_ID,
      clientSecret: c.GITHUB_CLIENT_SECRET,
      webUrl: c.GITHUB_WEB_URL,
      apiUrl: c.GITHUB_API_URL,
    },
    testAuthShortcut: shortcut,
    secureCookies: c.NODE_ENV === "production",
    kek: kek ? { key: kek, version: c.ROLERANK_KEK_VERSION } : null,
  };
}
