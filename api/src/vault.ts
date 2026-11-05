import { createCipheriv, createDecipheriv, randomBytes } from "node:crypto";
import type { Pool, PoolClient } from "pg";

/**
 * Application-layer encryption (CLAUDE.md, privacy invariants). Each user has a 256-bit data
 * key, stored wrapped by the key-encryption key, which lives outside the database (SSM in
 * production, .env locally). Every ciphertext is AES-256-GCM with additional data naming its
 * table, column and row, so a ciphertext copied to another row or user fails to open.
 *
 * Layout: version byte (1), 12-byte nonce, 16-byte tag, ciphertext.
 */
const VERSION = 1;

type Db = Pool | PoolClient;

export class Vault {
  constructor(
    private readonly kek: Buffer,
    private readonly kekVersion: number,
  ) {
    if (kek.length !== 32) throw new Error("the key-encryption key must be 32 bytes");
  }

  static seal(key: Buffer, plaintext: Buffer, aad: string): Buffer {
    const nonce = randomBytes(12);
    const cipher = createCipheriv("aes-256-gcm", key, nonce);
    cipher.setAAD(Buffer.from(aad));
    const body = Buffer.concat([cipher.update(plaintext), cipher.final()]);
    return Buffer.concat([Buffer.from([VERSION]), nonce, cipher.getAuthTag(), body]);
  }

  static open(key: Buffer, sealed: Buffer, aad: string): Buffer {
    if (sealed.length < 29 || sealed[0] !== VERSION) throw new Error("not a sealed value");
    const decipher = createDecipheriv("aes-256-gcm", key, sealed.subarray(1, 13));
    decipher.setAAD(Buffer.from(aad));
    decipher.setAuthTag(sealed.subarray(13, 29));
    return Buffer.concat([decipher.update(sealed.subarray(29)), decipher.final()]);
  }

  /** The user's data key, created on first use. */
  async userKey(db: Db, userId: string): Promise<Buffer> {
    const aad = `user_keys:${userId}`;
    const fresh = Vault.seal(this.kek, randomBytes(32), aad);
    await db.query(
      `INSERT INTO user_keys (user_id, wrapped, kek_version) VALUES ($1, $2, $3) ON CONFLICT (user_id) DO NOTHING`,
      [userId, fresh, this.kekVersion],
    );
    const { rows } = await db.query<{ wrapped: Buffer; kek_version: number }>(
      `SELECT wrapped, kek_version FROM user_keys WHERE user_id = $1`,
      [userId],
    );
    const row = rows[0];
    if (!row) throw new Error("user key missing");
    if (row.kek_version !== this.kekVersion) throw new Error(`user key wrapped by kek version ${row.kek_version}`);
    return Vault.open(this.kek, row.wrapped, aad);
  }

  /** Encrypts a value for one column of one row of one user. */
  async encrypt(db: Db, userId: string, column: string, rowId: string, value: unknown): Promise<Buffer> {
    const key = await this.userKey(db, userId);
    return Vault.seal(key, Buffer.from(JSON.stringify(value)), `${column}:${rowId}:${userId}`);
  }

  async decrypt<T>(db: Db, userId: string, column: string, rowId: string, sealed: Buffer): Promise<T> {
    const key = await this.userKey(db, userId);
    return JSON.parse(Vault.open(key, sealed, `${column}:${rowId}:${userId}`).toString("utf8")) as T;
  }
}
