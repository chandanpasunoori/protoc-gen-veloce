package main

const validateRuntimeSource = `export type Ignore = "IF_ZERO" | "ALWAYS";

export type StringFormat =
  | "email" | "hostname" | "ip" | "ipv4" | "ipv6" | "uri" | "uriRef"
  | "address" | "uuid" | "tuuid" | "hostAndPort" | "ulid";

export interface StringRuleSet {
  readonly const?: string;
  readonly len?: number;
  readonly minLen?: number;
  readonly maxLen?: number;
  readonly lenBytes?: number;
  readonly minBytes?: number;
  readonly maxBytes?: number;
  readonly pattern?: string;
  readonly prefix?: string;
  readonly suffix?: string;
  readonly contains?: string;
  readonly notContains?: string;
  readonly in?: readonly string[];
  readonly notIn?: readonly string[];
  readonly format?: StringFormat;
  readonly wellKnownRegex?: "httpHeaderName" | "httpHeaderValue";
}

export type Numeric = number | bigint;

export interface NumberRuleSet {
  readonly const?: Numeric;
  readonly lt?: Numeric;
  readonly lte?: Numeric;
  readonly gt?: Numeric;
  readonly gte?: Numeric;
  readonly in?: readonly Numeric[];
  readonly notIn?: readonly Numeric[];
  readonly finite?: boolean;
}

export interface EnumRuleSet {
  readonly const?: number;
  readonly definedOnly?: boolean;
  readonly in?: readonly number[];
  readonly notIn?: readonly number[];
  readonly values?: readonly string[];
  readonly numbers?: readonly number[];
}

export interface BytesRuleSet {
  readonly len?: number;
  readonly minLen?: number;
  readonly maxLen?: number;
}

export interface RepeatedRuleSet {
  readonly minItems?: number;
  readonly maxItems?: number;
  readonly unique?: boolean;
  readonly items?: FieldRuleSet;
}

export interface MapRuleSet {
  readonly minPairs?: number;
  readonly maxPairs?: number;
  readonly keys?: FieldRuleSet;
  readonly values?: FieldRuleSet;
}

export interface FieldRuleSet {
  readonly label: string;
  readonly required?: boolean;
  readonly ignore?: Ignore;
  readonly string?: StringRuleSet;
  readonly number?: NumberRuleSet;
  readonly enum?: EnumRuleSet;
  readonly bytes?: BytesRuleSet;
  readonly repeated?: RepeatedRuleSet;
  readonly map?: MapRuleSet;
}

export type MessageRules<T> = { readonly [K in keyof T & string]?: FieldRuleSet };

export interface Violation {
  readonly field: string;
  readonly rule: string;
  readonly message: string;
}

export interface MessageContext {
  readonly label: string;
  readonly value: unknown;
  readonly rule: string;
  readonly param?: unknown;
}

export interface ValidateOptions {
  readonly labels?: Readonly<Record<string, string>>;
  readonly messages?: Readonly<Record<string, (ctx: MessageContext) => string>>;
  readonly skip?: readonly string[];
  readonly ignoreEmpty?: boolean;
}

const FORMAT_PATTERNS: Record<StringFormat, RegExp> = {
  email: /^[^\s@]+@[^\s@]+\.[^\s@]+$/,
  hostname: /^(?=.{1,253}$)([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$/,
  ip: /^((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}|([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4})$/,
  ipv4: /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/,
  ipv6: /^([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}$/,
  uri: /^[a-zA-Z][a-zA-Z0-9+.-]*:[^\s]+$/,
  uriRef: /^[^\s]+$/,
  address: /^[^\s]+$/,
  uuid: /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/,
  tuuid: /^[0-9a-fA-F]{32}$/,
  hostAndPort: /^[^\s:]+:\d{1,5}$|^\[[0-9a-fA-F:]+\]:\d{1,5}$/,
  ulid: /^[0-7][0-9A-HJKMNP-TV-Za-hjkmnp-tv-z]{25}$/,
};

const FORMAT_LABELS: Record<StringFormat, string> = {
  email: "a valid email address",
  hostname: "a valid hostname",
  ip: "a valid IP address",
  ipv4: "a valid IPv4 address",
  ipv6: "a valid IPv6 address",
  uri: "a valid URI",
  uriRef: "a valid URI or path",
  address: "a valid hostname or IP address",
  uuid: "a valid UUID",
  tuuid: "a valid UUID",
  hostAndPort: "a valid host and port, like example.com:8080",
  ulid: "a valid ULID",
};

const WELL_KNOWN_REGEX: Record<"httpHeaderName" | "httpHeaderValue", { re: RegExp; label: string }> = {
  httpHeaderName: { re: /^:?[0-9a-zA-Z!#$%&'*+\-.^_|~\x60]+$/, label: "a valid HTTP header name" },
  httpHeaderValue: { re: /^[^\u0000-\u0008\u000A-\u001F\u007F]*$/, label: "a valid HTTP header value" },
};

const INTEGER = /^-?\d+$/;

function isZero(value: unknown, numeric: boolean): boolean {
  if (value === null || value === undefined || value === "") return true;
  if (typeof value === "number") return value === 0;
  if (typeof value === "bigint") return value === 0n;
  if (typeof value === "boolean") return !value;
  if (typeof value === "string") return numeric && INTEGER.test(value.trim()) && BigInt(value.trim()) === 0n;
  if (Array.isArray(value)) return value.length === 0;
  if (typeof value === "object") return Object.keys(value).length === 0;
  return false;
}

function toNumeric(value: unknown): Numeric | undefined {
  if (typeof value === "number") return value;
  if (typeof value === "bigint") return value;
  if (typeof value === "string") {
    const text = value.trim();
    if (INTEGER.test(text)) return BigInt(text);
    if (text !== "" && !Number.isNaN(Number(text))) return Number(text);
  }
  return undefined;
}

function sameNumber(a: Numeric, b: Numeric): boolean {
  if (typeof a === typeof b) return a === b;
  if (typeof a === "number" && !Number.isInteger(a)) return false;
  if (typeof b === "number" && !Number.isInteger(b)) return false;
  return BigInt(a) === BigInt(b);
}

function codePoints(value: string): number {
  return [...value].length;
}

function matches(pattern: string, value: string): boolean {
  for (const flags of ["u", ""]) {
    try {
      return new RegExp(pattern, flags).test(value);
    } catch {
      continue;
    }
  }
  return true;
}

function utf8Length(value: string): number {
  return new TextEncoder().encode(value).length;
}

function base64Length(value: string): number {
  const digits = value.replace(/[\s=]/g, "").length;
  return Math.floor((digits * 3) / 4);
}

function plural(n: number, one: string, many: string): string {
  return n === 1 ? one : many;
}

function list(values: readonly (string | Numeric)[]): string {
  return values.map(String).join(", ");
}

/** Validates one value against a field rule set, returning every violation found. */
export function validateValue(
  rules: FieldRuleSet,
  value: unknown,
  fieldPath: string,
  options?: ValidateOptions,
): Violation[] {
  const out: Violation[] = [];
  const label = options?.labels?.[fieldPath] ?? rules.label;

  const push = (rule: string, fallback: string, param?: unknown): void => {
    const override = options?.messages?.[rule];
    const message = override ? override({ label, value, rule, param }) : fallback;
    out.push({ field: fieldPath, rule, message });
  };

  if (rules.ignore === "ALWAYS") return out;

  const empty = isZero(value, rules.number !== undefined);
  if (rules.required && empty) {
    push("required", label + " is required.");
    return out;
  }
  if (empty && (rules.ignore === "IF_ZERO" || options?.ignoreEmpty)) return out;

  const s = rules.string;
  if (s && typeof value === "string") {
    const chars = codePoints(value);
    if (s.const !== undefined && value !== s.const) {
      push("string.const", label + " must be \"" + s.const + "\".", s.const);
    }
    if (s.len !== undefined && chars !== s.len) {
      push("string.len", label + " must be exactly " + s.len + " " + plural(s.len, "character", "characters") + ".", s.len);
    }
    if (s.minLen !== undefined && chars < s.minLen) {
      push("string.min_len", label + " must be at least " + s.minLen + " " + plural(s.minLen, "character", "characters") + ".", s.minLen);
    }
    if (s.maxLen !== undefined && chars > s.maxLen) {
      push("string.max_len", label + " must be at most " + s.maxLen + " " + plural(s.maxLen, "character", "characters") + ".", s.maxLen);
    }
    if (s.lenBytes !== undefined || s.minBytes !== undefined || s.maxBytes !== undefined) {
      const bytes = utf8Length(value);
      if (s.lenBytes !== undefined && bytes !== s.lenBytes) {
        push("string.len_bytes", label + " must be exactly " + s.lenBytes + " " + plural(s.lenBytes, "byte", "bytes") + ".", s.lenBytes);
      }
      if (s.minBytes !== undefined && bytes < s.minBytes) {
        push("string.min_bytes", label + " must be at least " + s.minBytes + " " + plural(s.minBytes, "byte", "bytes") + ".", s.minBytes);
      }
      if (s.maxBytes !== undefined && bytes > s.maxBytes) {
        push("string.max_bytes", label + " must be at most " + s.maxBytes + " " + plural(s.maxBytes, "byte", "bytes") + ".", s.maxBytes);
      }
    }
    if (s.prefix !== undefined && !value.startsWith(s.prefix)) {
      push("string.prefix", label + " must start with \"" + s.prefix + "\".", s.prefix);
    }
    if (s.suffix !== undefined && !value.endsWith(s.suffix)) {
      push("string.suffix", label + " must end with \"" + s.suffix + "\".", s.suffix);
    }
    if (s.contains !== undefined && !value.includes(s.contains)) {
      push("string.contains", label + " must contain \"" + s.contains + "\".", s.contains);
    }
    if (s.notContains !== undefined && value.includes(s.notContains)) {
      push("string.not_contains", label + " must not contain \"" + s.notContains + "\".", s.notContains);
    }
    if (s.in && s.in.length > 0 && !s.in.includes(value)) {
      push("string.in", label + " must be one of: " + list(s.in) + ".", s.in);
    }
    if (s.notIn && s.notIn.includes(value)) {
      push("string.not_in", label + " must not be one of: " + list(s.notIn) + ".", s.notIn);
    }
    if (s.format && !FORMAT_PATTERNS[s.format].test(value)) {
      push("string." + s.format, label + " must be " + FORMAT_LABELS[s.format] + ".", s.format);
    }
    if (s.wellKnownRegex) {
      const wk = WELL_KNOWN_REGEX[s.wellKnownRegex];
      if (!wk.re.test(value)) {
        push("string.well_known_regex", label + " must be " + wk.label + ".", s.wellKnownRegex);
      }
    }
    if (s.pattern !== undefined && !matches(s.pattern, value)) {
      push("string.pattern", label + " is not in the expected format.", s.pattern);
    }
  }

  const n = rules.number;
  const num = n ? toNumeric(value) : undefined;
  if (n && num !== undefined) {
    if (n.finite && typeof num === "number" && !Number.isFinite(num)) {
      push("number.finite", label + " must be a finite number.");
    } else if (typeof num === "bigint" || !Number.isNaN(num)) {
      if (n.const !== undefined && !sameNumber(num, n.const)) {
        push("number.const", label + " must be " + n.const + ".", n.const);
      }
      if (n.gte !== undefined && num < n.gte) {
        push("number.gte", label + " must be " + n.gte + " or more.", n.gte);
      }
      if (n.gt !== undefined && num <= n.gt) {
        push("number.gt", label + " must be greater than " + n.gt + ".", n.gt);
      }
      if (n.lte !== undefined && num > n.lte) {
        push("number.lte", label + " must be " + n.lte + " or less.", n.lte);
      }
      if (n.lt !== undefined && num >= n.lt) {
        push("number.lt", label + " must be less than " + n.lt + ".", n.lt);
      }
      if (n.in && n.in.length > 0 && !n.in.some((x) => sameNumber(num, x))) {
        push("number.in", label + " must be one of: " + list(n.in) + ".", n.in);
      }
      if (n.notIn && n.notIn.some((x) => sameNumber(num, x))) {
        push("number.not_in", label + " must not be one of: " + list(n.notIn) + ".", n.notIn);
      }
    }
  }

  const e = rules.enum;
  if (e && (typeof value === "string" || typeof value === "number")) {
    let asNumber: number | undefined;
    if (typeof value === "number") {
      asNumber = value;
    } else if (e.values && e.numbers) {
      const i = e.values.indexOf(value);
      asNumber = i >= 0 ? e.numbers[i] : undefined;
    }
    const known = typeof value === "string" ? (e.values?.includes(value) ?? false) : (e.numbers?.includes(value) ?? false);

    if (e.definedOnly && !known) {
      push("enum.defined_only", label + " is not a valid option.");
    }
    if (asNumber !== undefined) {
      if (e.const !== undefined && asNumber !== e.const) {
        push("enum.const", label + " must be a specific value.", e.const);
      }
      if (e.in && e.in.length > 0 && !e.in.includes(asNumber)) {
        push("enum.in", label + " is not an allowed option.", e.in);
      }
      if (e.notIn && e.notIn.includes(asNumber)) {
        push("enum.not_in", label + " is not an allowed option.", e.notIn);
      }
    }
  }

  const b = rules.bytes;
  if (b && (typeof value === "string" || value instanceof Uint8Array)) {
    const size = typeof value === "string" ? base64Length(value) : value.length;
    if (b.len !== undefined && size !== b.len) {
      push("bytes.len", label + " must be exactly " + b.len + " " + plural(b.len, "byte", "bytes") + ".", b.len);
    }
    if (b.minLen !== undefined && size < b.minLen) {
      push("bytes.min_len", label + " must be at least " + b.minLen + " " + plural(b.minLen, "byte", "bytes") + ".", b.minLen);
    }
    if (b.maxLen !== undefined && size > b.maxLen) {
      push("bytes.max_len", label + " must be at most " + b.maxLen + " " + plural(b.maxLen, "byte", "bytes") + ".", b.maxLen);
    }
  }

  const r = rules.repeated;
  if (r && Array.isArray(value)) {
    if (r.minItems !== undefined && value.length < r.minItems) {
      push("repeated.min_items", label + " must have at least " + r.minItems + " " + plural(r.minItems, "item", "items") + ".", r.minItems);
    }
    if (r.maxItems !== undefined && value.length > r.maxItems) {
      push("repeated.max_items", label + " must have at most " + r.maxItems + " " + plural(r.maxItems, "item", "items") + ".", r.maxItems);
    }
    if (r.unique && new Set(value.map((v) => JSON.stringify(v))).size !== value.length) {
      push("repeated.unique", label + " must not contain duplicates.");
    }
    const items = r.items;
    if (items) {
      value.forEach((item, i) => {
        out.push(...validateValue(items, item, fieldPath + "[" + i + "]", options));
      });
    }
  }

  const m = rules.map;
  if (m && typeof value === "object" && value !== null && !Array.isArray(value)) {
    const entries = Object.entries(value);
    if (m.minPairs !== undefined && entries.length < m.minPairs) {
      push("map.min_pairs", label + " must have at least " + m.minPairs + " " + plural(m.minPairs, "entry", "entries") + ".", m.minPairs);
    }
    if (m.maxPairs !== undefined && entries.length > m.maxPairs) {
      push("map.max_pairs", label + " must have at most " + m.maxPairs + " " + plural(m.maxPairs, "entry", "entries") + ".", m.maxPairs);
    }
    for (const [k, v] of entries) {
      if (m.keys) out.push(...validateValue(m.keys, k, fieldPath + "[" + JSON.stringify(k) + "]", options));
      if (m.values) out.push(...validateValue(m.values, v, fieldPath + "." + k, options));
    }
  }

  return out;
}

/** Validates one field of a message against its generated rules. */
export function validateField<T>(
  rules: MessageRules<T>,
  field: keyof T & string,
  value: unknown,
  options?: ValidateOptions,
): Violation[] {
  const fieldRules = rules[field];
  return fieldRules ? validateValue(fieldRules, value, field, options) : [];
}

/** Validates a whole message, or a partially filled form for it, against its generated rules. */
export function validate<T>(
  rules: MessageRules<T>,
  value: NoInfer<Partial<T>>,
  options?: ValidateOptions,
): Violation[] {
  const out: Violation[] = [];
  const skip = new Set(options?.skip);
  const record = value as Readonly<Record<string, unknown>>;
  for (const key of Object.keys(rules) as (keyof T & string)[]) {
    const fieldRules = rules[key];
    if (skip.has(key) || !fieldRules) continue;
    out.push(...validateValue(fieldRules, record[key], key, options));
  }
  return out;
}

/** Returns the first violation message per field, keyed by field path. */
export function violationMap(violations: readonly Violation[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const v of violations) {
    if (!(v.field in out)) out[v.field] = v.message;
  }
  return out;
}
`
