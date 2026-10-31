import { createHash, randomBytes, timingSafeEqual } from "node:crypto";

export function randomToken(bytes = 32): string {
  return randomBytes(bytes).toString("base64url");
}

export function sha256(value: string): Buffer {
  return createHash("sha256").update(value).digest();
}

/** PKCE S256 challenge for a verifier (RFC 7636). */
export function pkceChallenge(verifier: string): string {
  return sha256(verifier).toString("base64url");
}

/** Constant-time comparison. Hashing first hides the length of either value. */
export function safeEqual(a: string, b: string): boolean {
  return timingSafeEqual(sha256(a), sha256(b));
}
