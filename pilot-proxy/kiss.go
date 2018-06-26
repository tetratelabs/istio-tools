package pilotproxy

import (
	"fmt"
	"io"
	"log"

	ads "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
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

	for {
		// read the request and forward it downstream
		req, err := stream.Recv()
		if err != nil {
			if status.Code(err) == codes.Canceled || err == io.EOF {
				return nil
			}
			return err
		}
		if err := downstream.Send(req); err != nil {
			return err
		}

		// receive the response
		resp, err := downstream.Recv()
		if err != nil {
			if status.Code(err) == codes.Canceled || err == io.EOF {
				return nil
			}
			return err
		}

		// apply the plugins for this type of response
		for _, p := range s.plugins[typeStringToEvent(resp.TypeUrl)] {
			if err := p.Apply(resp); err != nil {
				return err
			}
		}

		// forward it back
		if err := stream.Send(resp); err != nil {
			return err
		}
	}

	return nil
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
