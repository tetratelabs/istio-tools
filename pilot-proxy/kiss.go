package pilotproxy

import (
	"fmt"
	"io"
	"log"

	"sync"

	ads "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// the Keep It Simple, Stupid implementation, which keeps every Envoy-downstream pair in its own single goroutine.
type server_kiss struct {
	client ads.AggregatedDiscoveryServiceClient

	plugins map[EventType][]Plugin
}

// Creates a new ADS proxy server that will apply all plugins to each response.
func NewKissServer(client ads.AggregatedDiscoveryServiceClient, plugins map[EventType][]Plugin) ads.AggregatedDiscoveryServiceServer {
	return &server_kiss{client, plugins}
}

func (s *server_kiss) StreamAggregatedResources(stream ads.AggregatedDiscoveryService_StreamAggregatedResourcesServer) error {
	// create a connection for this client
	downstream, err := s.client.StreamAggregatedResources(stream.Context())
	if err != nil {
		return fmt.Errorf("failed to open downstream connection: %v", err)
	}
	// and clean it up when we exit, logging any failures
	defer func() {
		if err := downstream.CloseSend(); err != nil {
			log.Printf("failed to close downstream with: %v", err)
		}
	}()

	// we've gotta do some book-keeping using data from the first request
	first := sync.Once{}
	for {
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
			if err := p.Apply(resp); err != nil {
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
func typeStringToEvent(tt string) EventType {
	switch EventType(tt) {
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
