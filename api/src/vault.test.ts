import { randomBytes } from "node:crypto";
import { describe, expect, it } from "vitest";
import { Vault } from "./vault.js";

describe("Vault.seal", () => {
  const key = randomBytes(32);

  it("round-trips", () => {
    const sealed = Vault.seal(key, Buffer.from("resume text"), "resumes.lines_ct:r1:u1");
    expect(Vault.open(key, sealed, "resumes.lines_ct:r1:u1").toString()).toBe("resume text");
  });

  it("refuses a ciphertext moved to another row or user", () => {
    const sealed = Vault.seal(key, Buffer.from("resume text"), "resumes.lines_ct:r1:u1");
    expect(() => Vault.open(key, sealed, "resumes.lines_ct:r2:u1")).toThrow();
    expect(() => Vault.open(key, sealed, "resumes.lines_ct:r1:u2")).toThrow();
  });

  it("refuses a tampered or foreign ciphertext", () => {
    const sealed = Vault.seal(key, Buffer.from("resume text"), "a");
    sealed.writeUInt8((sealed.at(-1) ?? 0) ^ 1, sealed.length - 1);
    expect(() => Vault.open(key, sealed, "a")).toThrow();
    expect(() => Vault.open(randomBytes(32), Vault.seal(key, Buffer.from("x"), "a"), "a")).toThrow();
  });

  it("never contains the plaintext", () => {
    const sealed = Vault.seal(key, Buffer.from("Kubernetes at Acme"), "a");
    expect(sealed.includes(Buffer.from("Kubernetes"))).toBe(false);
  });

  it("needs a 32-byte key-encryption key", () => {
    expect(() => new Vault(randomBytes(16), 1)).toThrow();
  });
});
