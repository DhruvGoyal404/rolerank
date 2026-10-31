import type { JsonWebKey } from "node:crypto";
import { z } from "zod";
import type { Config } from "../config.js";
import { pkceChallenge } from "../crypto.js";
import { verifyIdToken } from "./idtoken.js";

export type ProviderName = "google" | "github";

export interface AuthorizeInput {
  state: string;
  verifier: string;
  nonce: string;
  redirectUri: string;
}

export interface ExchangeInput {
  code: string;
  verifier: string;
  nonce: string;
  redirectUri: string;
}

export interface ProviderIdentity {
  subject: string;
  email: string | null;
  emailVerified: boolean;
}

export interface Provider {
  name: ProviderName;
  authorizeUrl(input: AuthorizeInput): Promise<string>;
  exchange(input: ExchangeInput): Promise<ProviderIdentity>;
}

const TIMEOUT_MS = 10_000;
const CACHE_MS = 3_600_000;

async function getJson(url: string, headers: Record<string, string> = {}): Promise<unknown> {
  const response = await fetch(url, {
    headers: { accept: "application/json", ...headers },
    redirect: "error",
    signal: AbortSignal.timeout(TIMEOUT_MS),
  });
  if (!response.ok) throw new Error(`provider responded ${response.status}`);
  return response.json();
}

async function postForm(url: string, form: Record<string, string>): Promise<unknown> {
  const response = await fetch(url, {
    method: "POST",
    headers: {
      accept: "application/json",
      "content-type": "application/x-www-form-urlencoded",
    },
    body: new URLSearchParams(form),
    redirect: "error",
    signal: AbortSignal.timeout(TIMEOUT_MS),
  });
  if (!response.ok) throw new Error(`provider responded ${response.status}`);
  return response.json();
}

const discoverySchema = z.object({
  authorization_endpoint: z.url(),
  token_endpoint: z.url(),
  jwks_uri: z.url(),
});

const jwksSchema = z.object({ keys: z.array(z.looseObject({ kid: z.string() })) });

export function googleProvider(config: Config["google"], now: () => number = Date.now): Provider {
  let discovery: { value: z.infer<typeof discoverySchema>; at: number } | undefined;
  let jwks: { keys: Map<string, JsonWebKey>; at: number } | undefined;

  async function discover(): Promise<z.infer<typeof discoverySchema>> {
    if (discovery && now() - discovery.at < CACHE_MS) return discovery.value;
    const url = `${config.issuer}/.well-known/openid-configuration`;
    const value = discoverySchema.parse(await getJson(url));
    discovery = { value, at: now() };
    return value;
  }

  async function loadKeys(): Promise<Map<string, JsonWebKey>> {
    const { jwks_uri: uri } = await discover();
    const body = jwksSchema.parse(await getJson(uri));
    const keys = new Map(body.keys.map((key) => [key.kid, key as JsonWebKey]));
    jwks = { keys, at: now() };
    return keys;
  }

  // Google rotates keys: an unknown kid forces one refresh before giving up.
  async function getKey(kid: string): Promise<JsonWebKey | undefined> {
    if (jwks && now() - jwks.at < CACHE_MS) {
      const cached = jwks.keys.get(kid);
      if (cached) return cached;
    }
    return (await loadKeys()).get(kid);
  }

  const hostless = config.issuer.replace(/^https:\/\//, "");

  return {
    name: "google",
    async authorizeUrl(input) {
      const { authorization_endpoint: endpoint } = await discover();
      const url = new URL(endpoint);
      url.search = new URLSearchParams({
        client_id: config.clientId,
        redirect_uri: input.redirectUri,
        response_type: "code",
        scope: "openid email",
        state: input.state,
        nonce: input.nonce,
        code_challenge: pkceChallenge(input.verifier),
        code_challenge_method: "S256",
      }).toString();
      return url.toString();
    },
    async exchange(input) {
      const { token_endpoint: endpoint } = await discover();
      const body = z.object({ id_token: z.string() }).parse(
        await postForm(endpoint, {
          grant_type: "authorization_code",
          code: input.code,
          client_id: config.clientId,
          client_secret: config.clientSecret,
          redirect_uri: input.redirectUri,
          code_verifier: input.verifier,
        }),
      );
      const claims = await verifyIdToken({
        token: body.id_token,
        getKey,
        issuers: [config.issuer, hostless],
        audience: config.clientId,
        nonce: input.nonce,
        nowSeconds: Math.floor(now() / 1000),
      });
      return {
        subject: claims.sub,
        email: claims.email ?? null,
        emailVerified: claims.email_verified === true,
      };
    },
  };
}

export function githubProvider(config: Config["github"]): Provider {
  return {
    name: "github",
    authorizeUrl(input) {
      const url = new URL(`${config.webUrl}/login/oauth/authorize`);
      url.search = new URLSearchParams({
        client_id: config.clientId,
        redirect_uri: input.redirectUri,
        scope: "read:user",
        state: input.state,
        code_challenge: pkceChallenge(input.verifier),
        code_challenge_method: "S256",
      }).toString();
      return Promise.resolve(url.toString());
    },
    async exchange(input) {
      // GitHub answers 200 with an error body, so success means access_token is present.
      const body = z.object({ access_token: z.string().min(1) }).parse(
        await postForm(`${config.webUrl}/login/oauth/access_token`, {
          code: input.code,
          client_id: config.clientId,
          client_secret: config.clientSecret,
          redirect_uri: input.redirectUri,
          code_verifier: input.verifier,
        }),
      );
      const user = z.object({ id: z.number() }).parse(
        await getJson(`${config.apiUrl}/user`, {
          authorization: `Bearer ${body.access_token}`,
          "x-github-api-version": "2022-11-28",
          "user-agent": "RoleRank",
        }),
      );
      return { subject: String(user.id), email: null, emailVerified: false };
    },
  };
}
