package main

const wktRuntimeSource = `const DURATION_RE = /^(-?(?:\d+)(?:\.\d+)?)s$/;

/** Parses an RFC 3339 timestamp string into a Date, or null when absent/invalid. */
export function parseTimestamp(value: string | null | undefined): Date | null {
  if (value === null || value === undefined || value === "") return null;
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? null : d;
}

/** Formats a Date as an RFC 3339 timestamp string, or null when absent/invalid. */
export function formatTimestamp(value: Date | number | null | undefined): string | null {
  if (value === null || value === undefined) return null;
  const d = typeof value === "number" ? new Date(value) : value;
  return Number.isNaN(d.getTime()) ? null : d.toISOString();
}

/** Reports whether a value parses as an RFC 3339 timestamp. */
export function isTimestamp(value: unknown): boolean {
  return typeof value === "string" && parseTimestamp(value) !== null;
}

/** Parses a protobuf duration string (e.g. "1.5s") into milliseconds. */
export function parseDuration(value: string | null | undefined): number | null {
  if (value === null || value === undefined || value === "") return null;
  const m = DURATION_RE.exec(value);
  if (!m || m[1] === undefined) return null;
  const seconds = Number(m[1]);
  return Number.isNaN(seconds) ? null : seconds * 1000;
}

/** Formats milliseconds as a protobuf duration string (e.g. "1.5s"). */
export function formatDuration(ms: number | null | undefined): string | null {
  if (ms === null || ms === undefined || Number.isNaN(ms)) return null;
  const seconds = ms / 1000;
  return (Number.isInteger(seconds) ? String(seconds) : seconds.toFixed(9).replace(/0+$/, "")) + "s";
}

/** Reports whether a value parses as a protobuf duration string. */
export function isDuration(value: unknown): boolean {
  return typeof value === "string" && parseDuration(value) !== null;
}

/** Splits a comma-separated FieldMask string into its paths. */
export function parseFieldMask(value: string | null | undefined): string[] {
  if (!value) return [];
  return value.split(",").map((p) => p.trim()).filter((p) => p.length > 0);
}

/** Joins FieldMask paths into the comma-separated wire form. */
export function formatFieldMask(paths: readonly string[] | null | undefined): string | null {
  if (!paths || paths.length === 0) return null;
  return paths.join(",");
}
`
