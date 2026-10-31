import pg from "pg";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { createTestDatabases, urlFor, type TestDatabases } from "./harness.js";

let dbs: TestDatabases;

beforeAll(async () => {
  dbs = await createTestDatabases();
});
afterAll(async () => {
  await dbs.drop();
});

async function canConnect(role: string, password: string | undefined, database: string): Promise<string> {
  const client = new pg.Client({ connectionString: urlFor(role, password ?? "", database) });
  try {
    await client.connect();
    await client.query("SELECT 1");
    return "connected";
  } catch (error) {
    return (error as { code?: string }).code ?? "unknown";
  } finally {
    await client.end().catch(() => undefined);
  }
}

const INSUFFICIENT_PRIVILEGE = "42501";

describe("database isolation", () => {
  it("lets each role reach its own database", async () => {
    const { appRw, corpusRw, corpusRo } = dbs.roles;
    expect(await canConnect(appRw, dbs.passwords.APP_RW, dbs.appDb)).toBe("connected");
    expect(await canConnect(corpusRw, dbs.passwords.CORPUS_RW, dbs.corpusDb)).toBe("connected");
    expect(await canConnect(corpusRo, dbs.passwords.CORPUS_RO, dbs.corpusDb)).toBe("connected");
  });

  it("refuses app_rw on corpus", async () => {
    expect(await canConnect(dbs.roles.appRw, dbs.passwords.APP_RW, dbs.corpusDb)).toBe(
      INSUFFICIENT_PRIVILEGE,
    );
  });

  it("refuses the corpus roles on app", async () => {
    const { corpusRw, corpusRo } = dbs.roles;
    expect(await canConnect(corpusRw, dbs.passwords.CORPUS_RW, dbs.appDb)).toBe(
      INSUFFICIENT_PRIVILEGE,
    );
    expect(await canConnect(corpusRo, dbs.passwords.CORPUS_RO, dbs.appDb)).toBe(
      INSUFFICIENT_PRIVILEGE,
    );
  });

  it("refuses each migrator on the other database", async () => {
    const { migratorApp, migratorCorpus } = dbs.roles;
    expect(await canConnect(migratorApp, dbs.passwords.MIGRATOR_APP, dbs.corpusDb)).toBe(
      INSUFFICIENT_PRIVILEGE,
    );
    expect(await canConnect(migratorCorpus, dbs.passwords.MIGRATOR_CORPUS, dbs.appDb)).toBe(
      INSUFFICIENT_PRIVILEGE,
    );
  });

  it("gives PUBLIC no CONNECT on either database", async () => {
    const client = new pg.Client({
      connectionString: urlFor(dbs.roles.appRw, dbs.passwords.APP_RW ?? "", dbs.appDb),
    });
    await client.connect();
    const { rows } = await client.query<{ name: string; can: boolean }>(
      `SELECT d.datname AS name, EXISTS (
         SELECT 1 FROM aclexplode(d.datacl) a WHERE a.grantee = 0 AND a.privilege_type = 'CONNECT'
       ) AS can
       FROM pg_database d WHERE d.datname = ANY($1)`,
      [[dbs.appDb, dbs.corpusDb]],
    );
    await client.end();
    expect(rows).toHaveLength(2);
    expect(rows.every((row) => !row.can)).toBe(true);
  });

  it("keeps runtime roles out of the schema", async () => {
    const client = new pg.Client({
      connectionString: urlFor(dbs.roles.appRw, dbs.passwords.APP_RW ?? "", dbs.appDb),
    });
    await client.connect();
    await expect(client.query("CREATE TABLE nope (id int)")).rejects.toMatchObject({
      code: INSUFFICIENT_PRIVILEGE,
    });
    await expect(client.query("DROP TABLE users")).rejects.toMatchObject({
      code: INSUFFICIENT_PRIVILEGE,
    });
    await client.end();
  });

  it("makes corpus_ro read-only", async () => {
    const client = new pg.Client({
      connectionString: urlFor(dbs.roles.corpusRo, dbs.passwords.CORPUS_RO ?? "", dbs.corpusDb),
    });
    await client.connect();
    await expect(client.query("SELECT count(*) FROM sources")).resolves.toBeTruthy();
    await expect(
      client.query("INSERT INTO sources (name) VALUES ('nope')"),
    ).rejects.toMatchObject({ code: INSUFFICIENT_PRIVILEGE });
    await client.end();
  });

  it("has no cross-database extensions, and no PII columns in corpus", async () => {
    const client = new pg.Client({
      connectionString: urlFor(dbs.roles.corpusRo, dbs.passwords.CORPUS_RO ?? "", dbs.corpusDb),
    });
    await client.connect();
    const extensions = await client.query<{ extname: string }>(`SELECT extname FROM pg_extension`);
    const names = extensions.rows.map((row) => row.extname);
    expect(names).toContain("vector");
    expect(names).not.toContain("dblink");
    expect(names).not.toContain("postgres_fdw");

    const columns = await client.query<{ table_name: string; column_name: string }>(
      `SELECT table_name, column_name FROM information_schema.columns WHERE table_schema = 'public'`,
    );
    expect(columns.rows.length).toBeGreaterThan(0);
    const pii = /email|phone|resume|profile|user_?id|session|password|address|contact/i;
    const hits = columns.rows.filter((row) => pii.test(row.column_name));
    expect(hits).toEqual([]);
    await client.end();
  });
});
