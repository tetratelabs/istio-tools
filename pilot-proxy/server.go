package pilotproxy

import (
	"context"
	"fmt"
	"io"
	golog "log"
	"net"
	"sync"

	xdscore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	ads "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	grpcmw "github.com/grpc-ecosystem/go-grpc-middleware"
	gprom "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/uber/jaeger-client-go/config"
	jprom "github.com/uber/jaeger-lib/metrics/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// the Keep It Simple, Stupid implementation, which keeps every Envoy-downstream pair in its own single goroutine.
type server struct {
	client ads.AggregatedDiscoveryServiceClient

	plugins map[string][]Plugin
}

// Creates a new ADS proxy server that will apply all plugins to each response.
func NewServer(client ads.AggregatedDiscoveryServiceClient, plugins map[string][]Plugin) ads.AggregatedDiscoveryServiceServer {
	return &server{client, plugins}
}

// Serve starts the server serving: it will connect to the downstream ADS server, listen on the provided address,
// and start to proxy from connecting clients to the downstream, applying plugins to the downstream's responses.
func Serve(downstreamAddress, listenAddress, serviceName string, plugins map[string][]Plugin) error {
	// set up jaeger, including exporting metrics to prometheus
	metricsFactory := jprom.New()
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
	conn, err := grpc.DialContext(ctx, downstreamAddress,
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
	ads.RegisterAggregatedDiscoveryServiceServer(s, NewServer(client, plugins))

	// and serve it
	l, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("failed to listen on")
	}

	golog.Printf("starting ADS server listening on %q", listenAddress)
	return s.Serve(l)
}

func (s *server) StreamAggregatedResources(stream ads.AggregatedDiscoveryService_StreamAggregatedResourcesServer) error {
	// create a connection for this client
	downstream, err := s.client.StreamAggregatedResources(stream.Context())
	if err != nil {
		return fmt.Errorf("failed to open downstream connection: %v", err)
	}
	// and clean it up when we exit, logging any failures
	defer func() {
		if err := downstream.CloseSend(); err != nil {
			golog.Printf("failed to close downstream with: %v", err)
		}
	}()

	// we've gotta do some book-keeping using data from the first request
	first := sync.Once{}
	for {
		var node *xdscore.Node
		nodeId := "error-on-first-connection"
		// read the request and forward it downstream
		req, err := stream.Recv()
		if err != nil {
			if status.Code(err) == codes.Canceled || err == io.EOF {
				return nil
			}
			requestsReceivedErrors.WithLabelValues(nodeId).Inc()
			return err
		}
		first.Do(func() {
			clientsConnected.Inc()
			defer clientsConnected.Dec()
			// set the value of the node ID for later iteration's counters. This is so we don't have to special case
			// incrementing the error counter when the error is the first message in the stream
			node = req.Node
			nodeId = req.Node.Id
			// we keep track of requests, errors per node connected; when the node disconnects we'll stop tracking it
			// we don't delete the error counters since we close the stream when we increment them
			defer requestsReceived.DeleteLabelValues(nodeId)
			defer requestsForwarded.DeleteLabelValues(nodeId)
			defer downstreamResponseReceived.DeleteLabelValues(nodeId)
			defer responseForwarded.DeleteLabelValues(nodeId)
		})

		requestsReceived.WithLabelValues(nodeId).Inc()
		if err := downstream.Send(req); err != nil {
			requestsForwardedErrors.WithLabelValues(nodeId).Inc()
			return err
		}

		// receive the response
		resp, err := downstream.Recv()
		downstreamResponseReceived.WithLabelValues(nodeId).Inc()
		if err != nil {
			if status.Code(err) == codes.Canceled || err == io.EOF {
				return nil
			}
			downstreamResponseReceivedErrors.WithLabelValues(nodeId).Inc()
			return err
		}

		// apply the plugins for this type of response
		for _, p := range s.plugins[typeStringToEvent(resp.TypeUrl)] {
			if err := p.Apply(node, resp); err != nil {
				pluginApplyError.WithLabelValues(resp.TypeUrl, p.Name())
				return err
			}
		}

		// forward it on
		responseForwarded.WithLabelValues(nodeId).Inc()
		if err := stream.Send(resp); err != nil {
			responseForwardedErrors.WithLabelValues(nodeId).Inc()
			return err
		}
	}
}

// converts Type URLs from Envoy requests into our EventType representation
func typeStringToEvent(tt string) string {
	switch tt {
	case ClusterType:
		return ClusterType
	case ListenerType:
		return ListenerType
	case EndpointType:
		return EndpointType
	case RouteType:
		return RouteType
	default:
		return "" // none
	}
}

var (
	clientsConnected = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "pilotproxy-ads_clients_connected",
		Help: "Number of clients connected to the proxy",
	})

	requestsReceived = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pilotproxy-ads_received",
		Help: "Number of ADS requests received from each node",
	}, []string{"node"})

	requestsReceivedErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pilotproxy-ads_received_err",
		Help: "Number of ADS receive errors by node",
	}, []string{"node"})

	requestsForwarded = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pilotproxy-ads_forwarded",
		Help: "Number of ADS requests forwarded to the downstream ADS server",
	}, []string{"node"})

	requestsForwardedErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pilotproxy-ads_forwarded_err",
		Help: "Number of ADS requests that failed to send to the downstream per node",
	}, []string{"node"})

	downstreamResponseReceived = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pilotproxy-ads_downstream_received",
		Help: "Number of ADS responses received from the downstream server",
	}, []string{"node"})

	downstreamResponseReceivedErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pilotproxy-ads_downstream_received_err",
		Help: "Number of ADS response errors received from the downstream server",
	}, []string{"node"})

	responseForwarded = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pilotproxy-ads_response_forwarded",
		Help: "Number of ADS responses forwarded per node",
	}, []string{"node"})

	responseForwardedErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pilotproxy-ads_response_forwarded_err",
		Help: "Number of ADS response forwarding errors per node",
	}, []string{"node"})

	pluginApplyError = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pilotproxy-ads_plugin_apply_error",
		Help: "Number of errors returned per plugin when invoked on an ADS request",
	}, []string{"type", "plugin"})
)
