/**
 * What each source requires of a job's credit and links (docs/SOURCES.md; the same rules as
 * worker/internal/source Attribution). Checked by attribution.test.ts against each rule.
 *   Greenhouse, Lever: "via ..." as honest provenance; link to the employer's posting.
 *   Himalayas: credit, and the apply link must be their applicationLink, unchanged.
 *   Remote OK: a plain followable link (no nofollow) to the posting on Remote OK, the name
 *   credited, and never their logo.
 */
export interface SourceRule {
  credit: string;
  /** rel for the posting link: Remote OK's terms ask for a followable one. */
  rel: string;
  /** The apply link is the source's own, used exactly as stored. */
  exactApply: boolean;
}

const RULES: Record<string, SourceRule> = {
  greenhouse: { credit: "Greenhouse", rel: "noopener noreferrer", exactApply: false },
  lever: { credit: "Lever", rel: "noopener noreferrer", exactApply: false },
  himalayas: { credit: "Himalayas", rel: "noopener noreferrer", exactApply: true },
  remoteok: { credit: "Remote OK", rel: "noopener", exactApply: false },
};

export function ruleFor(source: string): SourceRule {
  return RULES[source] ?? { credit: source, rel: "noopener noreferrer", exactApply: false };
}

/** Where "Apply" goes: the source's apply link where it gives one, else the posting itself. */
export function applyHref(job: { url: string; applyUrl: string | null; source: string }): string {
  return job.applyUrl ?? job.url;
}

export const SOURCES = Object.keys(RULES);
