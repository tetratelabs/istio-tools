# gRPC-JSON Transcoder Config Generator

A simple utility to generate an Istio [EnvoyFilter](https://preliminary.istio.io/docs/reference/config/istio.networking.v1alpha3/#EnvoyFilter) CRD configuring Envoy's [gRPC-JSON transcoder filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http_filters/grpc_json_transcoder_filter). This filter allows a gRPC server to serve JSON HTTP requests without any additional work on the server's part, regardless of the language the gRPC server is written in. Envoy transcodes the client JSON requests into gRPC before handing the request to the server, and transcodes the server's response from gRPC back into JSON before sending it to the client. Even better, it can do this while still serving gRPC clients on the same port!

## Usage

1. Build by binary via `make build` which creates a binary named `gen-transcoder`.
    > Alternatively, use `go run github.com/tetratelabs/istio-tools/grpc-transcoder` and pass the same CLI arguments as we use in the other examples.

1. Build your protobuf API definitions with [`protoc`](https://github.com/google/protobuf/releases), instructing the compiler to produce _descriptors_, a binary file that describes the runtime format of protobufs alongside their metadata. These descriptors are used to perform transocoding at runtime. Note that your gRPC service's need to use `google.api.http` options to describe their mapping to a REST API.

    > [Google's Cloud Endpoints' documentation](https://cloud.google.com/endpoints/docs/grpc/transcoding) provides an overview of using these proto/gRPC features, as well as how generate descriptors.
  
    ```sh
    protoc \
      -I path/to/google/protobufs \
      -I path/to/your/protos \
      --descriptor_set_out=path/to/output/dir/YOUR_SERVICE_NAME.proto-descriptor \
      --include_imports \
      --go_out=plugins=grpc:. \
      path/to/your/protos/service.proto
    ```
  
1. Note the fully qualified name of your gRPC service's protobuf, i.e. `proto.package.name.Service`. You may choose to provide prefix of matching package names e.g. `--packages proto.package`. If none provided, all packages will be chosen, and listed.

1. You may choose to match services using comma-separated regular expressions e.g. `--services http.*,echo.*`. If none provided, all services will be chosen and listed.

1. Find the `app` label of the Kubernetes Service you want to enable transcoding for. For our example, we'll assume our Kubernetes uses the label `app: foo`.

1. Note the port your gRPC server is running on; for our example we'll assume the gRPC server listens on port `9080`.

1. Use `gen-transcoder` to generate your configuration for Istio:

    ```sh
    gen-transcoder \
      --port 9080 \
      [--service foo] \
      [--packages proto.package.name] \
      [--services 'Service.*'] \
      --descriptor path/to/output/dir/YOUR_SERVICE_NAME.proto-descriptor
    ```

     Which will spit out config looking like:
  
    ```yaml
    # Created by github.com/tetratelabs/istio-tools/grpc-transcoder
    apiVersion: networking.istio.io/v1alpha3
    kind: EnvoyFilter
    metadata:
      name: foo
    spec:
      workloadLabels:
        app: foo
      filters:
      - listenerMatch:
          portNumber: 9080 
          listenerType: SIDECAR_INBOUND
        insertPosition:
          index: BEFORE
          relativeTo: envoy.router
        filterName: envoy.grpc_json_transcoder
        filterType: HTTP
        filterConfig:
          protoDescriptorBin: <Base 64 Encoded String, the binary data inside of path/to/output/dir/YOUR_SERVICE_NAME.proto-descriptor>
          services:
          - proto.package.name.Service1
          - proto.package.name.Service2
          printOptions:
            alwaysPrintPrimitiveFields: True
    ```
-------

We have included a few [sample proto services](/grpc-transcoder/protos), compiled into a single proto descriptor that you can use in the following way:

```sh
gen-transcoder \
  --port 9080 \
  --service echo \
  --packages proto \
  --services 'Echo.*' \
  --descriptor proto/onebig.proto-descriptor
```

Which spits out the config below:

```yaml
# Created by github.com/tetratelabs/istio-tools/grpc-transcoder
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: echo
spec:
  workloadLabels:
    app: echo
  filters:
  - listenerMatch:
      portNumber: 9080
      listenerType: SIDECAR_INBOUND
    insertPosition:
      index: BEFORE
      relativeTo: envoy.router
    filterName: envoy.grpc_json_transcoder
    filterType: HTTP
    filterConfig:
      services:
      - proto.EchoService
      protoDescriptorBin: Cs0BCgplY2hvLnByb3RvEgVwcm90byIxCgtFY2hvUmVxdWVzdBIOCgJpZBgBIAEoCVICaWQSEgoEYm9keRgCIAEoDFIEYm9keSIyCgxFY2hvUmVzcG9uc2USDgoCaWQYASABKAlSAmlkEhIKBGJvZHkYAiABKAxSBGJvZHkyQAoLRWNob1NlcnZpY2USMQoERWNobxISLnByb3RvLkVjaG9SZXF1ZXN0GhMucHJvdG8uRWNob1Jlc3BvbnNlIgBCB1oFcHJvdG9iBnByb3RvMwrvAQoQaGVsbG93b3JsZC5wcm90bxIKaGVsbG93b3JsZCIiCgxIZWxsb1JlcXVlc3QSEgoEbmFtZRgBIAEoCVIEbmFtZSImCgpIZWxsb1JlcGx5EhgKB21lc3NhZ2UYASABKAlSB21lc3NhZ2UySQoHR3JlZXRlchI+CghTYXlIZWxsbxIYLmhlbGxvd29ybGQuSGVsbG9SZXF1ZXN0GhYuaGVsbG93b3JsZC5IZWxsb1JlcGx5IgBCMAobaW8uZ3JwYy5leGFtcGxlcy5oZWxsb3dvcmxkQg9IZWxsb1dvcmxkUHJvdG9QAWIGcHJvdG8zCswBCgp0ZXN0LnByb3RvEgVwcm90byIxCgtUZXN0UmVxdWVzdBIOCgJpZBgBIAEoCVICaWQSEgoEYm9keRgCIAEoDFIEYm9keSIyCgxUZXN0UmVzcG9uc2USDgoCaWQYASABKAlSAmlkEhIKBGJvZHkYAiABKAxSBGJvZHkyPwoLVGVzdFNlcnZpY2USMAoDR2V0EhIucHJvdG8uVGVzdFJlcXVlc3QaEy5wcm90by5UZXN0UmVzcG9uc2UiAEIHWgVwcm90b2IGcHJvdG8z
      printOptions:
        alwaysPrintPrimitiveFields: True
```

#### TODO: full example including protos, k8s service + deployment definition showing full e2e setup
