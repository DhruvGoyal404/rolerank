/**
 * The one way pages talk to the API: same origin, JSON in and out. A failed call throws an
 * ApiError carrying the status and the API's fixed error code, never a message from the server
 * shown as HTML (everything renders as text).
 */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
  ) {
    super(code);
  }
}

export async function api<T>(path: string, init: { method?: string; body?: unknown; raw?: Blob } = {}): Promise<T> {
  const headers: Record<string, string> = {};
  let body: BodyInit | null = null;
  if (init.raw) {
    body = init.raw;
    headers["content-type"] = init.raw.type || "application/octet-stream";
  } else if (init.body !== undefined) {
    body = JSON.stringify(init.body);
    headers["content-type"] = "application/json";
  }
  const res = await fetch(path, { method: init.method ?? "GET", headers, body, credentials: "same-origin" });
  if (res.status === 204) return undefined as T;
  const data = (await res.json().catch(() => ({}))) as { error?: string };
  if (!res.ok) throw new ApiError(res.status, data.error ?? "error");
  return data as T;
}

// Shapes the API returns (api/src/http).
export interface Me {
  id: string;
  resume: { id: string; status: string; error: string | null } | null;
  profile: { id: string; confirmed: boolean } | null;
}

export interface Explanation {
  matched: string[];
  missing: string[];
  niceMatched: string[];
  level: { job: string; profile: string; fit: "fits" | "stretch" | "above" | "unknown" };
}

export interface JobCard {
  key: string;
  title: string;
  company: string;
  location: string | null;
  level: string;
  url: string;
  applyUrl: string | null;
  excerpt: string;
  source: string;
  closed: boolean;
  postedAt: string;
}

export interface Match {
  job: JobCard;
  band: string;
  explanation: Explanation;
}

export interface ShownLine {
  n: number;
  text: string;
}

export interface Tailoring {
  outcome: string;
  evidence: Array<{ skill: string; lines: Array<ShownLine & { reason: string | null }> }>;
  notShown: string[];
  alignments: Array<{ line: number; from: string; to: string; rewrite: string }>;
  emphasis: { top: ShownLine[]; addMetric: ShownLine[] };
}

export interface TrackerItem {
  id: string;
  key: string;
  title: string;
  company: string;
  url: string;
  status: string;
  updatedAt: string;
}

export interface Profile {
  level: "intern" | "entry" | "mid" | "senior";
  skills: Array<{ name: string; line: number }>;
  experience: Array<{ employer: string; title: string; kind: string; periods: Array<{ start: string; end: string }>; lines: number[] }>;
  education: Array<{ institution: string; degree: string; grad_year: number | null; score: string; lines: number[] }>;
  projects: Array<{ name: string; line: number }>;
  publications: Array<{ venue: string; title: string; date: string; lines: number[] }>;
  achievements: Array<{ text: string; lines: number[] }>;
  certifications: Array<{ name: string; lines: number[] }>;
}
