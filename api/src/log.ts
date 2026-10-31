import { pino, type DestinationStream, type Logger } from "pino";

/**
 * Only these fields ever reach a log line. It's an allowlist so a new field can't
 * leak by accident: resume text, profile fields and tokens have no way in.
 */
const ALLOWED = ["event", "method", "route", "status", "ms", "code", "provider"] as const;
type Field = (typeof ALLOWED)[number];
export type LogFields = Partial<Record<Field, string | number>>;

export interface Log {
  info(fields: LogFields): void;
  error(fields: LogFields): void;
}

export function pick(fields: Record<string, unknown>): LogFields {
  const out: LogFields = {};
  for (const key of ALLOWED) {
    const value = fields[key];
    if (typeof value === "string" || typeof value === "number") out[key] = value;
  }
  return out;
}

export function createLog(destination?: DestinationStream): Log {
  const logger: Logger = pino({ base: null }, destination);
  return {
    info: (fields) => {
      logger.info(pick(fields));
    },
    error: (fields) => {
      logger.error(pick(fields));
    },
  };
}
