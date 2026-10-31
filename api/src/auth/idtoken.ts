import { createPublicKey, verify, type JsonWebKey } from "node:crypto";
import { z } from "zod";
import { safeEqual } from "../crypto.js";

const headerSchema = z.object({ alg: z.string(), kid: z.string().optional() });

const claimsSchema = z.object({
  iss: z.string(),
  aud: z.string(),
  sub: z.string().min(1),
  exp: z.number(),
  iat: z.number().optional(),
  nonce: z.string().optional(),
  email: z.string().optional(),
  email_verified: z.boolean().optional(),
});

export type IdTokenClaims = z.infer<typeof claimsSchema>;

export interface VerifyOptions {
  token: string;
  getKey: (kid: string) => Promise<JsonWebKey | undefined>;
  issuers: readonly string[];
  audience: string;
  nonce: string;
  nowSeconds: number;
}

export class IdTokenError extends Error {
  override name = "IdTokenError";
}

function decodeJson(part: string): unknown {
  try {
    return JSON.parse(Buffer.from(part, "base64url").toString("utf8"));
  } catch {
    throw new IdTokenError("malformed");
  }
}

/**
 * Verifies a Google ID token by hand. The algorithm is fixed to RS256 before any key
 * is touched, so `alg: none` and HS256 key confusion can't get through.
 */
export async function verifyIdToken(options: VerifyOptions): Promise<IdTokenClaims> {
  const parts = options.token.split(".");
  const [headerPart, payloadPart, signaturePart] = parts;
  if (parts.length !== 3 || !headerPart || !payloadPart || !signaturePart) {
    throw new IdTokenError("malformed");
  }

  const header = headerSchema.safeParse(decodeJson(headerPart));
  if (!header.success) throw new IdTokenError("malformed");
  if (header.data.alg !== "RS256") throw new IdTokenError("algorithm");
  if (!header.data.kid) throw new IdTokenError("key");

  const jwk = await options.getKey(header.data.kid);
  if (!jwk) throw new IdTokenError("key");

  const signed = verify(
    "RSA-SHA256",
    Buffer.from(`${headerPart}.${payloadPart}`),
    createPublicKey({ key: jwk, format: "jwk" }),
    Buffer.from(signaturePart, "base64url"),
  );
  if (!signed) throw new IdTokenError("signature");

  const claims = claimsSchema.safeParse(decodeJson(payloadPart));
  if (!claims.success) throw new IdTokenError("malformed");
  const c = claims.data;

  if (!options.issuers.includes(c.iss)) throw new IdTokenError("issuer");
  if (c.aud !== options.audience) throw new IdTokenError("audience");
  if (c.exp <= options.nowSeconds) throw new IdTokenError("expired");
  if (c.nonce === undefined || !safeEqual(c.nonce, options.nonce)) {
    throw new IdTokenError("nonce");
  }
  return c;
}
