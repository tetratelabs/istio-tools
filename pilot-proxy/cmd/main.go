package main

import (
	"fmt"
	golog "log"
	"net"

	ads "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	grpcmw "github.com/grpc-ecosystem/go-grpc-middleware"
	gprom "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/uber/jaeger-lib/metrics/prometheus"
	"google.golang.org/grpc"

	"context"

	"github.com/tetratelabs/istio-tools/pilot-proxy"
	"github.com/tetratelabs/istio-tools/pilot-proxy/log"
	"github.com/uber/jaeger-client-go/config"
)

// Add new plugins here to wire them up at runtime.
var plugins = map[pilotproxy.EventType][]pilotproxy.Plugin{
	pilotproxy.ClusterType: {
		log.Plugin{},
	},
	pilotproxy.EndpointType: {
		log.Plugin{},
	},
	pilotproxy.RouteType: {
		log.Plugin{},
	},
	pilotproxy.ListenerType: {
		log.Plugin{},
	},
}

func main() {
	// TODO: flags to disable jaeger/prom
	var (
		downstream    string
		listenAddress string
		serviceName   string
	)
	pflag.StringVarP(&downstream, "downstream", "d", "istio-pilot.istio-system.svc.cluster.local:15001",
		"The address, host:port, of the downstream ADS server we're proxying to.")
	pflag.StringVarP(&listenAddress, "listen", "l", ":29000",
		"The address this server will listen on; e.g. :29000. If no host is provided then the server will listen across of the host's IP addresses.")
	pflag.StringVarP(&serviceName, "name", "n", "pilot-proxy", "The name of this service for tracing, metrics.")

	cmd := &cobra.Command{
		Short:   "pilot-proxy",
		Example: "pilot-proxy --downstream istio-pilot.istio-system.svc.cluster.local:15001 --listen :9090",
		RunE: func(cmd *cobra.Command, args []string) error {
			return serve(downstream, listenAddress, serviceName)
		},
	}

	if err := cmd.Execute(); err != nil {
		golog.Fatal(err)
	}
}

func serve(downstream, listenAddress, serviceName string) error {
	// set up jaeger, including exporting metrics to prometheus
	metricsFactory := prometheus.New()
	tracer, closer, err := config.Configuration{
		ServiceName: serviceName,
	}.NewTracer(
		config.Metrics(metricsFactory),
	)
	defer closer.Close()
	if err != nil {
		golog.Fatal(err)
	}

	ctx := context.Background()
	// connect to the downstream and create a client for it
	// wire up tracing/propagation and prometheus metrics using grpc interceptors
	conn, err := grpc.DialContext(ctx, downstream,
		grpc.WithInsecure(),
		grpc.WithUnaryInterceptor(
			grpcmw.ChainUnaryClient(
				otgrpc.OpenTracingClientInterceptor(tracer),
				gprom.UnaryClientInterceptor,
			)),
		grpc.WithStreamInterceptor(
			grpcmw.ChainStreamClient(
				otgrpc.OpenTracingStreamClientInterceptor(tracer),
				gprom.StreamClientInterceptor),
		),
	)
	if err != nil {
		return err
	}
	client := ads.NewAggregatedDiscoveryServiceClient(conn)

	// wire up our serving of the ADS API, using tracing and metrics interceptors here too
	s := grpc.NewServer(
		grpc.UnaryInterceptor(
			grpcmw.ChainUnaryServer(
				otgrpc.OpenTracingServerInterceptor(tracer),
				gprom.UnaryServerInterceptor,
			)),
		grpc.StreamInterceptor(
			grpcmw.ChainStreamServer(
				otgrpc.OpenTracingStreamServerInterceptor(tracer),
				gprom.StreamServerInterceptor,
			)),
	)
	ads.RegisterAggregatedDiscoveryServiceServer(s, pilotproxy.NewKissServer(client, plugins))

	// and serve it
	l, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("failed to listen on")
	}

	golog.Printf("starting ADS server listening on %q", listenAddress)
	return s.Serve(l)
}
