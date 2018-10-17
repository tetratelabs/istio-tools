Istio Tools
=========
Tools that Tetrate has developed to smooth over Istio's rough spots.

Tools in the repo:
---------
- [**grpc-transcoder**](/grpc-transcoder): use Istio's sidecar as a ["gRPC Gateway"](https://github.com/grpc-ecosystem/grpc-gateway). Takes a proto descriptor and produces the Istio configuration to enable Envoy's gRPC-to-JSON trancoding filter.
