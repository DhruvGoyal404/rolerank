import { canonical } from "./ai/match.js";
import type { Profile } from "./ai/profile.js";

/**
 * The API's side of the ranker service (worker/cmd/ranker): it sends what search needs from the
 * confirmed profile, never resume lines or contact details, and treats any failure as
 * "unavailable".
 */
export interface Hit {
  id: number;
  band: string; // Strong, Good or Stretch: a fit band, never a percentage
}

export interface Ranker {
  search(q: RankQuery): Promise<Hit[] | null>;
  /** One posting's fit, ranked as a search would rank it. Undefined when the ranker is down. */
  score(q: RankQuery, id: number): Promise<Hit | null | undefined>;
}

export interface RankQuery {
  skills: string[];
  titles: string;
  level: string;
  country: string;
  remote: boolean;
  text: string;
  limit: number;
}

/** What search reads from a profile: canonical skills, the latest titles, level and a short text. */
export function rankQuery(p: Profile, opts: { country: string; remote: boolean; limit: number }): RankQuery {
  const skills = canonical(p.skills.map((s) => s.name));
  const titles = [...new Set(p.experience.map((e) => e.title).filter(Boolean))].slice(0, 2).join(", ");
  const text = [`${p.level} candidate.`, titles ? `Roles: ${titles}.` : "", `Skills: ${skills.join(", ")}.`].filter(Boolean).join(" ");
  return { skills, titles, level: p.level, text: text.slice(0, 4000), ...opts };
}

export function rankerClient(url: string, token: string, timeoutMs = 10_000): Ranker {
  const post = async (path: string, body: unknown): Promise<unknown> => {
    try {
      const res = await fetch(`${url}${path}`, {
        method: "POST",
        headers: { authorization: `Bearer ${token}`, "content-type": "application/json" },
        body: JSON.stringify(body),
        signal: AbortSignal.timeout(timeoutMs),
      });
      return res.ok ? await res.json() : undefined;
    } catch {
      return undefined;
    }
  };
  return {
    async search(q) {
      const body = (await post("/search", q)) as { hits?: Array<{ id: number; band: string }> } | undefined;
      return body ? (body.hits ?? []).map((h) => ({ id: h.id, band: h.band })) : null;
    },
    async score(q, id) {
      const body = (await post("/score", { ...q, id })) as { hit?: { id: number; band: string } | null } | undefined;
      if (!body) return undefined;
      return body.hit ? { id: body.hit.id, band: body.hit.band } : null;
    },
  };
}
