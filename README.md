# protoc-gen-veloce

A `protoc` / `buf` plugin that generates a dependency-free TypeScript client for gRPC services exposed
through [grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway). It reads the
`google.api.http` annotations in your `.proto` files and emits one typed function per RPC that calls
the matching REST endpoint, plus plain TypeScript types for every message and enum.

## Features

- **Zero runtime dependencies.** The small runtime (`veloce_client.ts`) is generated next to your
  code. There is nothing to install from npm.
- **Tree-shakeable RPC functions.** Each RPC is a standalone function such as
  `getBook(client, request)`, so a bundle only contains the endpoints the app actually calls.
- **Faithful grpc-gateway routing.** Path templates (`{name=shelves/*/books/*}`, nested `{book.id}`)
  are URL-encoded. `body: "*"`, `body: "field"` and body-less bindings are handled the way
  grpc-gateway parses them, with the remaining fields sent as query parameters (repeated as
  `?tag=a&tag=b`, nested messages as `filter.query=x`). `response_body`, custom verbs, server
  streaming and `google.api.HttpBody` downloads are supported.
- **Types that match the wire.** Fields use their JSON names. 64-bit integers and bytes are
  `string`. Well-Known Types map to their canonical JSON form (`Timestamp` → `string`, `Struct` →
  JSON object, …). Field presence follows what grpc-gateway actually emits (see
  [Field presence](#field-presence)).
- **Enums with helpers.** Each enum is a const object plus a string-union type, with number and
  label maps, an ordered `Values` list, `Options` ready to bind to a `<select>`, an `is*` guard and a
  `to*` coercion that accepts either JSON form.
- **Client-side validation from `protovalidate`.** `buf.validate` field rules become plain data
  (`<Message>Rules`) checked by a shared `veloce_validate.ts` runtime that produces human-readable
  messages. It works with any framework.
- **Modern TypeScript.** The output passes `strict`, `exactOptionalPropertyTypes`,
  `noUncheckedIndexedAccess`, `isolatedDeclarations`, `erasableSyntaxOnly` and
  `verbatimModuleSyntax`. Every import is type-only, and the code runs unchanged under Node's
  built-in type stripping.

## Installation

```bash
go install github.com/chandanpasunoori/protoc-gen-veloce@latest
```

## Usage

### With buf

```yaml
version: v2
plugins:
  - local: protoc-gen-veloce
    out: src/gen
    strategy: all
```

```bash
buf generate
```

`strategy: all` runs the plugin once over every file. It is required for `index=true` and gives the
most accurate `field_presence=split` classification. With buf's default per-directory strategy the
runtime files are emitted by every invocation; buf keeps one copy, which is harmless because they
are identical.

### With protoc

```bash
protoc \
  --plugin=protoc-gen-veloce=$(which protoc-gen-veloce) \
  --veloce_out=./src/gen \
  -I proto \
  proto/library/v1/*.proto
```

`go_package` is not required, and output paths always mirror the proto paths
(`library/v1/library.proto` → `library/v1/library.ts`). Dependency files are generated only when
your types reference them, so `google/type/date.proto` gets a module when a field uses
`google.type.Date`, while option-only imports such as `google/api/annotations.proto` or
`buf/validate/validate.proto` produce nothing.

## Calling RPCs

```typescript
import { VeloceClient, VeloceError } from "./gen/veloce_client.js";
import { getBook, listBooks, updateBook } from "./gen/library/v1/library.js";

const client = new VeloceClient({ baseUrl: "/api" });

const book = await getBook(client, { name: "shelves/1/books/2" });
const page = await listBooks(client, { pageSize: 20, filter: { author: "Le Guin" } });
await updateBook(client, { book: { name: book.name, title: "New title" }, updateMask: "title" });
```

Fields bound to the URL path are required by the function signature, so omitting one is a
compile error. RPCs whose request has no required fields can be called without a request
(`listBooks(client)`).

`VeloceClientConfig`:

| Field | Purpose |
|---|---|
| `baseUrl` | Prefix for every path. May be relative (`/api`) in browsers; must be absolute in Node. |
| `fetch` | Custom `fetch`, e.g. for tests or SSR. Defaults to the global one. |
| `interceptors` | `(request: Request) => Request \| Promise<Request>` hooks, run in order. Use them for auth headers, tracing or logging. |
| `debug` | Sends the `X-Grpc-Method` header when the code was generated with `debug_method_header=true`. |

Every function also takes `RequestOptions` (`headers`, `signal`) as its last argument.

Non-2xx responses throw `VeloceError` with `status` (HTTP), `code` (gRPC status code from the
gateway's error body), `message`, `details` and `error` (the parsed body).

Server-streaming RPCs return an async generator over grpc-gateway's newline-delimited stream:

```typescript
for await (const event of watchBooks(client, { shelf: "shelves/1" })) {
  console.log(event.title);
}
```

RPCs returning `google.api.HttpBody` resolve to the raw `Response`, so you can stream or `blob()`
the content. Client-streaming and bidirectional RPCs are skipped because they cannot be called with
`fetch`.

## Generated types

### Field presence

grpc-gateway's default marshaler sets `EmitUnpopulated`, so a response carries every field. The
exceptions are unset oneof members, which are omitted, and unset messages and proto2 `optional`
fields, which are sent as `null`. The `field_presence` option decides how much of that the types
assume:

| Mode | Request-side messages | Response-only messages |
|---|---|---|
| `split` *(default)* | every field optional; message fields also `\| null` | required, except oneof members (optional); message and proto2 `optional` fields are `T \| null` |
| `optional` | every field optional | every field optional |
| `required` | same as response-only | required, except oneof members; message fields `T \| null` |

A message counts as response-only when it is reachable from RPC outputs and never from inputs. A
message used both ways is treated as a request, because over-requiring a response field is a
runtime hazard while over-requiring a request field only costs a compile error. In every mode, a
field marked `(buf.validate.field).required = true` or proto2 `required` is required and non-null,
and a proto3 `optional` field is optional.

If you register a marshaler with `EmitUnpopulated` disabled, use `field_presence=optional`.

### Enums

```proto
import "google/protobuf/descriptor.proto";

extend google.protobuf.EnumValueOptions {
  string label = 50001;
}

enum Genre {
  GENRE_UNSPECIFIED = 0;
  GENRE_SCI_FI = 1 [(label) = "Science Fiction"];
  // label: Non-fiction
  GENRE_NON_FICTION = 2;
  GENRE_POETRY = 3;
}
```

```typescript
import { Genre, GenreLabel, GenreOptions, toGenre } from "./gen/library/v1/library.js";

Genre.GENRE_SCI_FI;          // "GENRE_SCI_FI"
GenreLabel.GENRE_SCI_FI;     // "Science Fiction"
GenreLabel.GENRE_POETRY;     // "Poetry"
GenreOptions;                // [{ value: "GENRE_SCI_FI", label: "Science Fiction" }, …], zero value excluded
toGenre(2);                  // "GENRE_NON_FICTION"
```

Labels resolve in this order: a custom enum-value option (see `label_option`), then a `label:` line
in the value's leading comment, then the value name with the enum prefix stripped and Title Cased.
Without its option, `GENRE_SCI_FI` would get the derived label `"Sci Fi"`.

## Options

Pass options with `opt` in `buf.gen.yaml`, or `--veloce_opt=key=value,…` with protoc. Unknown keys
and invalid values fail the run.

| Option | Values | Default | Effect |
|---|---|---|---|
| `field_presence` | `split`, `optional`, `required` | `split` | See [Field presence](#field-presence). |
| `label_option` | extension name | `label` | Name of a `string` extension on `google.protobuf.EnumValueOptions` used for enum labels. It is discovered by name in your protos; the plugin ships no `.proto`. |
| `import_extension` | `js`, `ts`, `none` | `js` | Extension on relative imports. Use `ts` for Node type stripping or `rewriteRelativeImportExtensions`, `none` for bundlers that resolve bare paths. |
| `index` | `true`, `false` | `false` | Emit `index.ts` re-exporting the runtime and every requested file. Generation fails if two modules export the same name. Requires `strategy: all`. |
| `debug_method_header` | `true`, `false` | `false` | Embed each RPC's fully-qualified gRPC name so a client built with `debug: true` sends it as `X-Grpc-Method`. Leave it off for production builds so internal package and service names stay out of the bundle. |

## Validation

Messages with `buf.validate` rules get a `<Message>Rules` descriptor:

```typescript
import { validate, validateField, violationMap } from "./gen/veloce_validate.js";
import { CreateBookRequestRules } from "./gen/library/v1/library.js";

validate(CreateBookRequestRules, { title: "", pageCount: 0 });
// [{ field: "title", rule: "required", message: "Title is required." },
//  { field: "pageCount", rule: "number.gte", message: "Page Count must be 1 or more." }]

validateField(CreateBookRequestRules, "isbn", "not an isbn");
violationMap(validate(CreateBookRequestRules, form));   // { [field]: firstMessage }
```

`validate` accepts a partial object, so it works on forms as they are being filled in.
`ValidateOptions` can override labels, replace messages per rule
(`messages: { "string.min_len": ({ label, param }) => … }`), skip fields, or set
`ignoreEmpty: true` for progressive validation. `ignore = IGNORE_IF_ZERO_VALUE` is honoured.

Supported rules: `required`, `ignore`; string length (in characters and in UTF-8 bytes), pattern,
prefix, suffix, contains, `in`, well-known formats (email, hostname, IP, URI, UUID, ULID,
host:port, …) and header regexes; numeric `const`/`lt`/`lte`/`gt`/`gte`/`in`/`not_in`/`finite` for
every width (64-bit values are compared exactly as `bigint`); enum `defined_only`/`const`/`in`; bytes
length; repeated items, uniqueness and counts; map key/value rules and pair counts.

The browser checks are a convenience; the server remains the enforcement point. CEL expressions,
message-level rules and oneof rules are not emitted, and rules are not applied recursively to nested
messages.

## Output

| File | When |
|---|---|
| `<proto path>.ts` | Every requested file, plus each dependency whose types are referenced |
| `veloce_client.ts` | Any file has a callable RPC |
| `veloce_validate.ts` | Any message has validation rules |
| `veloce_wkt.ts` | Timestamp, Duration or FieldMask is used. Provides parse and format helpers |
| `index.ts` | `index=true` |

The generated code targets ES2022 (`lib` must include `es2022` and `dom`, or `@types/node` for
`fetch` types) and TypeScript 5.4 or newer. It is checked against TypeScript 7.

## Development

```bash
make build      # go build
make install    # go install
make test       # buf generate against the local test protos
```
