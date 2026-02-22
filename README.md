# protoc-gen-veloce

A `protoc` plugin designed for [grpc](https://github.com/grpc/grpc) and [grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway). It opinionated TypeScript client meant to be used on the frontend to talk with your backend.

If you use `grpc-gateway`, you know it exposes a REST interface to your gRPC services. This plugin generates a clean, dependency-free TypeScript client that directly hits those REST paths while maintaining the exact message structures and service methods defined in your `.proto` files.

## Features


- **REST-Native Routes:** Automatically resolves path variables (`{id}`) from the `google.api.http` annotations into standard JS template literals. 
- **Native TypeScript Types:** Emits native TypeScript definitions (interfaces and type aliases) rather than bloated runtime wrappers.
- **Well-Known Type (WKT) JSON Mapping:** Standard types like `google.protobuf.Struct` and `google.protobuf.Timestamp` are generated natively as mapped values (e.g. `export type Timestamp = string;`) corresponding directly to how `grpc-gateway` serializes them into JSON.
- **Nested Types:** Full support for recursively generated nested message definitions using standard protobuf structural conventions (`Message_NestedMessage`).
- **Interceptors:** The generated client is designed to be easily wrapped with custom interceptors for features like logging, retries, or authentication.

## Installation

Make sure your environment has Go installed and configured.

```bash
# Clone the repository and build the plugin
make build

# Install the binary into your PATH
make install
```

## Usage

This generator works equally well with standard `protoc` or the modern `buf` toolchain.

### With Buf (Recommended)

Add the plugin to your `buf.gen.yaml` pipeline (in the newer v2 format):

```yaml
version: v2
plugins:
  - local: protoc-gen-veloce
    out: ./gen
    opt: paths=source_relative
```

Then simply generate:

```bash
buf generate
```

### With Protoc

```bash
protoc \
  --plugin=protoc-gen-veloce=$(which protoc-gen-veloce) \
  --veloce_out=./gen \
  --veloce_opt=paths=source_relative \
  YOUR_PROTO_FILES.proto
```

## How It Works Under The Hood

The generated service methods directly depend on a simple client instance called `VeloceClient` (imported from `@vlce/veloce-client`). 

When setting up your frontend, initialize the client, feed it your base gateway URL, and pass it to the generated classes to manage the REST bindings:

```typescript
import { MyServiceClient } from "./gen/my_service.js";
import { VeloceClient } from "@vlce/veloce-client";

// Setup your transport layer
const client = new VeloceClient({ baseUrl: "https://api.myproject.com" });

// Initialize the generated SDK
const mySvc = new MyServiceClient(client);

// Call standard proto methods that map to REST beneath the hood
const res = await mySvc.GetUser({ id: "123" });
console.log(res.name);
```

## Development

Use the included `Makefile` to quickly build and test the plugin locally against the included `test.proto`.

```bash
# Runs does an integration run via `buf` locally
make test
```
