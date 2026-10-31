import { createHmac, createSign, generateKeyPairSync, type JsonWebKey } from "node:crypto";
import { describe, expect, it } from "vitest";
import { IdTokenError, verifyIdToken } from "./idtoken.js";

const { publicKey, privateKey } = generateKeyPairSync("rsa", { modulusLength: 2048 });
const other = generateKeyPairSync("rsa", { modulusLength: 2048 });
const jwk = publicKey.export({ format: "jwk" });

const NOW = 1_800_000_000;
const NONCE = "nonce-value";
const AUD = "client-id";
const ISS = "https://accounts.google.com";

const b64 = (value: unknown): string => Buffer.from(JSON.stringify(value)).toString("base64url");

function sign(claims: object, header: object = { alg: "RS256", kid: "k1" }, key = privateKey): string {
  const body = `${b64(header)}.${b64(claims)}`;
  return `${body}.${createSign("RSA-SHA256").update(body).sign(key).toString("base64url")}`;
}

const good = { iss: ISS, aud: AUD, sub: "123", exp: NOW + 60, nonce: NONCE };

const verify = (token: string): ReturnType<typeof verifyIdToken> =>
  verifyIdToken({
    token,
    getKey: (kid) => Promise.resolve<JsonWebKey | undefined>(kid === "k1" ? jwk : undefined),
    issuers: [ISS, "accounts.google.com"],
    audience: AUD,
    nonce: NONCE,
    nowSeconds: NOW,
  });

async function rejection(token: string): Promise<string> {
  const error: unknown = await verify(token).catch((e: unknown) => e);
  expect(error).toBeInstanceOf(IdTokenError);
  return (error as Error).message;
}

describe("verifyIdToken", () => {
  it("accepts a valid token", async () => {
    await expect(verify(sign(good))).resolves.toMatchObject({ sub: "123" });
  });

  it("accepts the issuer without a scheme, as Google documents", async () => {
    await expect(verify(sign({ ...good, iss: "accounts.google.com" }))).resolves.toBeTruthy();
  });

  it("rejects alg=none with an empty signature", async () => {
    const token = `${b64({ alg: "none", kid: "k1" })}.${b64(good)}.`;
    expect(await rejection(token)).toBe("malformed");
  });

  it("rejects alg=none with a dummy signature", async () => {
    const token = `${b64({ alg: "none", kid: "k1" })}.${b64(good)}.AAAA`;
    expect(await rejection(token)).toBe("algorithm");
  });

  it("rejects HS256 signed with the public key (key confusion)", async () => {
    const body = `${b64({ alg: "HS256", kid: "k1" })}.${b64(good)}`;
    const pem = publicKey.export({ format: "pem", type: "spki" });
    const signature = createHmac("sha256", pem).update(body).digest("base64url");
    expect(await rejection(`${body}.${signature}`)).toBe("algorithm");
  });

  it("rejects an unsigned token", async () => {
    expect(await rejection(`${b64({ alg: "RS256", kid: "k1" })}.${b64(good)}`)).toBe("malformed");
  });

  it("rejects a signature from another key", async () => {
    const token = sign(good, { alg: "RS256", kid: "k1" }, other.privateKey);
    expect(await rejection(token)).toBe("signature");
  });

  it("rejects a tampered payload", async () => {
    const [header, , signature] = sign(good).split(".");
    const token = `${header ?? ""}.${b64({ ...good, sub: "attacker" })}.${signature ?? ""}`;
    expect(await rejection(token)).toBe("signature");
  });

  it("rejects an unknown key id", async () => {
    expect(await rejection(sign(good, { alg: "RS256", kid: "nope" }))).toBe("key");
  });

  it("rejects a missing key id", async () => {
    expect(await rejection(sign(good, { alg: "RS256" }))).toBe("key");
  });

  it("rejects the wrong audience", async () => {
    expect(await rejection(sign({ ...good, aud: "other" }))).toBe("audience");
  });

  it("rejects the wrong issuer", async () => {
    expect(await rejection(sign({ ...good, iss: "https://evil.example" }))).toBe("issuer");
  });

  it("rejects an expired token", async () => {
    expect(await rejection(sign({ ...good, exp: NOW - 1 }))).toBe("expired");
  });

  it("rejects a token expiring this exact second", async () => {
    expect(await rejection(sign({ ...good, exp: NOW }))).toBe("expired");
  });

  it("rejects a wrong nonce", async () => {
    expect(await rejection(sign({ ...good, nonce: "replayed" }))).toBe("nonce");
  });

  it("rejects a missing nonce", async () => {
    const { nonce: _nonce, ...claims } = good;
    expect(await rejection(sign(claims))).toBe("nonce");
  });

  it("rejects garbage", async () => {
    expect(await rejection("not.a.jwt")).toBe("malformed");
  });
});
