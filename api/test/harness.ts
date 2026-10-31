import { createSign, generateKeyPairSync, randomBytes } from "node:crypto";
import { readFileSync, readdirSync } from "node:fs";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { join } from "node:path";
import { Redis } from "iovalkey";
import pg from "pg";

const repoRoot = join(import.meta.dirname, "..", "..");

function required(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is required for integration tests`);
  return value;
}

export const adminUrl = (): string => required("ROLERANK_TEST_ADMIN_URL");
export const valkeyUrl = (): string => required("ROLERANK_TEST_VALKEY_URL");

export function urlFor(role: string, password: string, database: string): string {
  const url = new URL(adminUrl());
  url.username = role;
  url.password = password;
  url.pathname = `/${database}`;
  return url.toString();
}

export interface TestDatabases {
  prefix: string;
  appDb: string;
  corpusDb: string;
  passwords: Record<string, string>;
  roles: { migratorApp: string; migratorCorpus: string; appRw: string; corpusRw: string; corpusRo: string };
  drop(): Promise<void>;
}

async function admin<T>(database: string, run: (client: pg.Client) => Promise<T>): Promise<T> {
  const url = new URL(adminUrl());
  url.pathname = `/${database}`;
  const client = new pg.Client({ connectionString: url.toString() });
  await client.connect();
  try {
    return await run(client);
  } finally {
    await client.end();
  }
}

function migrationsUp(dir: string): string[] {
  return readdirSync(dir)
    .filter((file) => file.endsWith(".sql"))
    .sort()
    .map((file) => {
      const text = readFileSync(join(dir, file), "utf8");
      return (text.split("-- migrate:down")[0] ?? "").replace("-- migrate:up", "");
    });
}

/** Provisions throwaway roles and databases from the same templates production uses. */
export async function createTestDatabases(): Promise<TestDatabases> {
  const id = randomBytes(4).toString("hex");
  const prefix = `rrt${id}_`;
  const appDb = `rrt${id}_app_test`;
  const corpusDb = `rrt${id}_corpus_test`;
  const pw = (): string => randomBytes(16).toString("hex");
  const passwords = {
    MIGRATOR_APP: pw(),
    MIGRATOR_CORPUS: pw(),
    APP_RW: pw(),
    CORPUS_RW: pw(),
    CORPUS_RO: pw(),
  };

  const render = (file: string): string => {
    let sql = readFileSync(join(repoRoot, "db", "provision", file), "utf8")
      .replaceAll("__P__", prefix)
      .replaceAll("__APP_DB__", appDb)
      .replaceAll("__CORPUS_DB__", corpusDb);
    for (const [name, value] of Object.entries(passwords)) {
      sql = sql.replaceAll(`__PW_${name}__`, value);
    }
    return sql;
  };

  await admin("postgres", async (client) => {
    // CREATE DATABASE can't run in the implicit transaction of a multi-statement query.
    const statements = render("cluster.sql.tmpl")
      .split("\n")
      .filter((line) => !line.startsWith("--"))
      .join("\n")
      .split(";")
      .map((statement) => statement.trim())
      .filter(Boolean);
    for (const statement of statements) await client.query(statement);
  });
  await admin(appDb, (client) => client.query(render("app.sql.tmpl")));
  await admin(corpusDb, (client) => client.query(render("corpus.sql.tmpl")));

  const migrate = async (role: string, password: string, db: string, dir: string): Promise<void> => {
    const client = new pg.Client({ connectionString: urlFor(role, password, db) });
    await client.connect();
    for (const sql of migrationsUp(join(repoRoot, "db", dir, "migrations"))) await client.query(sql);
    await client.end();
  };
  // Migrators own their database but have no CONNECT grant of their own to rely on: owners do.
  await migrate(`${prefix}migrator_app`, passwords.MIGRATOR_APP, appDb, "app");
  await migrate(`${prefix}migrator_corpus`, passwords.MIGRATOR_CORPUS, corpusDb, "corpus");

  return {
    prefix,
    appDb,
    corpusDb,
    passwords,
    roles: {
      migratorApp: `${prefix}migrator_app`,
      migratorCorpus: `${prefix}migrator_corpus`,
      appRw: `${prefix}app_rw`,
      corpusRw: `${prefix}corpus_rw`,
      corpusRo: `${prefix}corpus_ro`,
    },
    drop: () =>
      admin("postgres", async (client) => {
        await client.query(`DROP DATABASE IF EXISTS ${appDb} WITH (FORCE)`);
        await client.query(`DROP DATABASE IF EXISTS ${corpusDb} WITH (FORCE)`);
        for (const role of ["migrator_app", "migrator_corpus", "app_rw", "corpus_rw", "corpus_ro"]) {
          await client.query(`DROP ROLE IF EXISTS ${prefix}${role}`);
        }
      }),
  };
}

export function testValkey(): Redis {
  return new Redis(valkeyUrl());
}

export interface FakeProvider {
  issuer: string;
  webUrl: string;
  apiUrl: string;
  /** What the next Google token exchange returns as claims. Undefined fields keep defaults. */
  googleClaims: Record<string, unknown>;
  googleSub: string;
  githubId: number;
  githubTokenBody: Record<string, unknown>;
  requests: { path: string; body: string }[];
  close(): Promise<void>;
}

export const GOOGLE_CLIENT_ID = "google-client";

/** A local stand-in for Google and GitHub, speaking just enough of both protocols. */
export async function startFakeProvider(): Promise<FakeProvider> {
  const { publicKey, privateKey } = generateKeyPairSync("rsa", { modulusLength: 2048 });
  const jwk = { ...publicKey.export({ format: "jwk" }), kid: "k1", alg: "RS256", use: "sig" };
  let lastNonce = "";

  const fake: FakeProvider = {
    issuer: "",
    webUrl: "",
    apiUrl: "",
    googleClaims: {},
    googleSub: "google-subject",
    githubId: 4242,
    githubTokenBody: { access_token: "gh-token" },
    requests: [],
    close: () => Promise.resolve(),
  };

  const idToken = (): string => {
    const header = { alg: "RS256", kid: "k1" };
    const claims = {
      iss: fake.issuer,
      aud: GOOGLE_CLIENT_ID,
      sub: fake.googleSub,
      exp: Math.floor(Date.now() / 1000) + 300,
      nonce: lastNonce,
      email: "person@example.com",
      email_verified: true,
      ...fake.googleClaims,
    };
    const encode = (v: unknown): string => Buffer.from(JSON.stringify(v)).toString("base64url");
    const body = `${encode(header)}.${encode(claims)}`;
    const signature = createSign("RSA-SHA256").update(body).sign(privateKey).toString("base64url");
    return `${body}.${signature}`;
  };

  const server: Server = createServer((req, res) => {
    let body = "";
    req.on("data", (chunk: Buffer) => (body += chunk.toString()));
    req.on("end", () => {
      const url = new URL(req.url ?? "/", fake.issuer);
      fake.requests.push({ path: url.pathname, body });
      const json = (value: unknown): void => {
        res.writeHead(200, { "content-type": "application/json" });
        res.end(JSON.stringify(value));
      };
      switch (url.pathname) {
        case "/.well-known/openid-configuration":
          json({
            authorization_endpoint: `${fake.issuer}/o/authorize`,
            token_endpoint: `${fake.issuer}/o/token`,
            jwks_uri: `${fake.issuer}/o/jwks`,
          });
          return;
        case "/o/jwks":
          json({ keys: [jwk] });
          return;
        case "/o/token":
          json({ id_token: idToken() });
          return;
        case "/login/oauth/access_token":
          json(fake.githubTokenBody);
          return;
        case "/user":
          json({ id: fake.githubId, login: "someone" });
          return;
        case "/nonce":
          lastNonce = url.searchParams.get("value") ?? "";
          json({});
          return;
        default:
          res.writeHead(404);
          res.end();
      }
    });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const { port } = server.address() as AddressInfo;
  fake.issuer = `http://127.0.0.1:${port}`;
  fake.webUrl = fake.issuer;
  fake.apiUrl = fake.issuer;
  fake.close = () =>
    new Promise((resolve) => {
      server.close(() => {
        resolve();
      });
      server.closeAllConnections();
    });
  return fake;
}
