package main

const runtimeClientSource = `export const VELOCE_DEBUG_METHOD_HEADER = "X-Grpc-Method";

export interface RequestOptions {
  headers?: HeadersInit;
  signal?: AbortSignal;
}

export type Interceptor = (request: Request) => Request | Promise<Request>;

export interface VeloceClientConfig {
  baseUrl?: string;
  fetch?: (request: Request) => Promise<Response>;
  interceptors?: readonly Interceptor[];
  debug?: boolean;
}

export interface Rpc {
  readonly method: string;
  readonly path: string;
  readonly body?: string;
  readonly grpcMethod?: string;
}

export class VeloceError extends Error {
  readonly status: number;
  readonly code: number | undefined;
  readonly details: readonly unknown[];
  readonly error: unknown;

  constructor(status: number, error: unknown, message: string) {
    super(message);
    this.name = "VeloceError";
    this.status = status;
    this.error = error;
    const body = isRecord(error) ? error : undefined;
    const code = body?.["code"];
    const details = body?.["details"];
    this.code = typeof code === "number" ? code : undefined;
    this.details = Array.isArray(details) ? details : [];
  }
}

export class VeloceClient {
  readonly #baseUrl: string;
  readonly #fetch: (request: Request) => Promise<Response>;
  readonly #interceptors: readonly Interceptor[];
  readonly #debug: boolean;

  constructor(config: VeloceClientConfig = {}) {
    this.#baseUrl = (config.baseUrl ?? "").replace(/\/+$/, "");
    const fetchFn = config.fetch ?? (typeof globalThis.fetch === "function" ? (request: Request) => globalThis.fetch(request) : undefined);
    if (fetchFn === undefined) {
      throw new TypeError("VeloceClient: no fetch implementation available");
    }
    this.#fetch = fetchFn;
    this.#interceptors = config.interceptors ?? [];
    this.#debug = config.debug ?? false;
  }

  async unary<T>(rpc: Rpc, request: object, options?: RequestOptions): Promise<T> {
    const response = await this.#send(rpc, request, options);
    const text = await response.text();
    if (!response.ok) {
      throw errorFrom(response, text);
    }
    return (text === "" ? {} : JSON.parse(text)) as T;
  }

  async *stream<T>(rpc: Rpc, request: object, options?: RequestOptions): AsyncGenerator<T, void, undefined> {
    const response = await this.#send(rpc, request, options);
    if (!response.ok) {
      throw errorFrom(response, await response.text());
    }
    if (response.body === null) {
      return;
    }
    const reader = response.body.pipeThrough(new TextDecoderStream()).getReader();
    let buffer = "";
    let finished = false;
    try {
      while (!finished) {
        const { done, value } = await reader.read();
        finished = done;
        buffer += value ?? "";
        let newline = buffer.indexOf("\n");
        while (newline >= 0 || (finished && buffer.trim() !== "")) {
          const end = newline >= 0 ? newline : buffer.length;
          const line = buffer.slice(0, end).trim();
          buffer = buffer.slice(end + 1);
          if (line !== "") {
            yield streamChunk<T>(response.status, line);
          }
          newline = buffer.indexOf("\n");
        }
      }
    } finally {
      if (!finished) {
        await reader.cancel().catch(() => undefined);
      }
    }
  }

  async raw(rpc: Rpc, request: object, options?: RequestOptions): Promise<Response> {
    const response = await this.#send(rpc, request, options);
    if (!response.ok) {
      throw errorFrom(response, await response.text());
    }
    return response;
  }

  async #send(rpc: Rpc, request: object, options: RequestOptions | undefined): Promise<Response> {
    const bound = new Set<string>();
    const path = expandPath(rpc.path, request, bound);
    const query = new URLSearchParams();
    let body: string | undefined;

    if (rpc.body === "*") {
      body = JSON.stringify(request);
    } else {
      if (rpc.body !== undefined) {
        bound.add(rpc.body);
        const value = lookup(request, rpc.body);
        if (value !== undefined) {
          body = JSON.stringify(value);
        }
      }
      appendQuery(query, request, "", bound);
    }

    const headers = new Headers(options?.headers);
    if (!headers.has("Accept")) {
      headers.set("Accept", "application/json");
    }
    if (body !== undefined && !headers.has("Content-Type")) {
      headers.set("Content-Type", "application/json");
    }
    if (this.#debug && rpc.grpcMethod !== undefined) {
      headers.set(VELOCE_DEBUG_METHOD_HEADER, rpc.grpcMethod);
    }

    const init: RequestInit = { method: rpc.method, headers };
    if (body !== undefined) {
      init.body = body;
    }
    if (options?.signal !== undefined) {
      init.signal = options.signal;
    }

    const search = query.toString();
    let req = new Request(this.#baseUrl + path + (search === "" ? "" : "?" + search), init);
    for (const interceptor of this.#interceptors) {
      req = await interceptor(req);
    }
    const fetchFn = this.#fetch;
    return fetchFn(req);
  }
}

const PATH_VARIABLE = /\{([^}=]+)(=[^}]*)?\}/g;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function lookup(source: unknown, path: string): unknown {
  let current = source;
  for (const key of path.split(".")) {
    if (!isRecord(current)) {
      return undefined;
    }
    current = current[key];
  }
  return current;
}

function expandPath(template: string, request: object, bound: Set<string>): string {
  return template.replace(PATH_VARIABLE, (_match: string, field: string, pattern: string | undefined) => {
    const value = lookup(request, field);
    if (value === undefined || value === null || value === "") {
      throw new TypeError("VeloceClient: missing path parameter \"" + field + "\"");
    }
    bound.add(field);
    const text = String(value);
    return pattern === undefined ? encodeURIComponent(text) : text.split("/").map(encodeURIComponent).join("/");
  });
}

function appendQuery(query: URLSearchParams, value: unknown, key: string, skip: Set<string>): void {
  if (value === undefined || value === null || value === "" || skip.has(key)) {
    return;
  }
  if (Array.isArray(value)) {
    for (const item of value) {
      if (item !== undefined && item !== null && item !== "" && typeof item !== "object") {
        query.append(key, String(item));
      }
    }
    return;
  }
  if (isRecord(value)) {
    for (const [name, child] of Object.entries(value)) {
      appendQuery(query, child, key === "" ? name : key + "." + name, skip);
    }
    return;
  }
  query.append(key, String(value));
}

function messageOf(value: unknown): string | undefined {
  const message = isRecord(value) ? value["message"] : undefined;
  return typeof message === "string" && message !== "" ? message : undefined;
}

function errorFrom(response: Response, text: string): VeloceError {
  let parsed: unknown = text === "" ? undefined : text;
  try {
    parsed = text === "" ? undefined : JSON.parse(text);
  } catch {
    parsed = text;
  }
  const fallback = text !== "" && typeof parsed === "string" ? text : response.statusText || "HTTP " + response.status;
  return new VeloceError(response.status, parsed, messageOf(parsed) ?? fallback);
}

function streamChunk<T>(status: number, line: string): T {
  const parsed: unknown = JSON.parse(line);
  if (isRecord(parsed)) {
    const error = parsed["error"];
    if (error !== undefined && error !== null) {
      throw new VeloceError(status, error, messageOf(error) ?? "stream error");
    }
    return parsed["result"] as T;
  }
  return parsed as T;
}
`
