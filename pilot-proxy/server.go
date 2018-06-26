package pilotproxy

import (
	"errors"
	"fmt"
	"io"
	"log"

	xds "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	ads "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type server struct {
	client ads.AggregatedDiscoveryServiceClient

	plugins map[EventType][]Plugin
}

// Creates a new ADS proxy server that will apply all plugins to each response.
func NewServer(client ads.AggregatedDiscoveryServiceClient, plugins map[EventType][]Plugin) ads.AggregatedDiscoveryServiceServer {
	return &server{client, plugins}
}

func (s *server) StreamAggregatedResources(stream ads.AggregatedDiscoveryService_StreamAggregatedResourcesServer) error {
	// create a connection for this client
	d, err := s.client.StreamAggregatedResources(stream.Context())
	if err != nil {
		return fmt.Errorf("failed to open downstream connection: %v", err)
	}
	downstream := newDownstream(d)
	defer downstream.Close()

	// start draining requests, forwarding them to the downstream
	upstream := drain(stream)
	defer upstream.Close()
	go forward(downstream.req, upstream.req)

	for resp := range downstream.resp {
		for _, p := range s.plugins[typeStringToEvent(resp.TypeUrl)] {
			if err := p.Apply(resp); err != nil {
				// TODO: should we exit the entire stream, or something less severe?
				return err
			}
		}
		upstream.resp <- resp
	}
	return nil
}

// A client that sends requests and expects responses
type upstream struct {
	io.Closer

	req    <-chan *xds.DiscoveryRequest
	recErr error // will be set when rec is closed if an error was encountered

	resp    chan<- *xds.DiscoveryResponse
	sendErr error // will be set when resp is closed is an error was encountered
}

// A server that we send requests to and wait for responses from.
type downstream struct {
	io.Closer

	req    chan<- *xds.DiscoveryRequest
	recErr error // will be set when rec is closed if an error was encountered

	resp    <-chan *xds.DiscoveryResponse
	sendErr error // will be set when resp is closed is an error was encountered
}

func forward(down chan<- *xds.DiscoveryRequest, up <-chan *xds.DiscoveryRequest) {
	for {
		req, more := <-up
		if !more || req == nil {
			return
		}
		down <- req
	}
}

func drain(stream ads.AggregatedDiscoveryService_StreamAggregatedResourcesServer) *upstream {
	req := make(chan *xds.DiscoveryRequest)
	resp := make(chan *xds.DiscoveryResponse)
	c := &upstream{
		req:  req,
		resp: resp,
	}
	// start a goroutine to drain the stream coming in, which will push
	go c.drain(stream, req)
	go c.forward(stream, resp)
	return c
}

// drains the stream into c.req's request channel.
func (c *upstream) drain(stream ads.AggregatedDiscoveryService_StreamAggregatedResourcesServer, out chan<- *xds.DiscoveryRequest) {
	defer close(out)

	for {
		req, err := stream.Recv()
		if err != nil {
			if status.Code(err) == codes.Canceled || err == io.EOF {
				return
			}
			c.recErr = err
			return
		}
		out <- req
	}
}

// serializes writes to stream through in
func (c *upstream) forward(stream ads.AggregatedDiscoveryService_StreamAggregatedResourcesServer, in <-chan *xds.DiscoveryResponse) {
	for {
		select {
		case resp, more := <-in:
			if !more || resp == nil {
				return
			}
			if err := stream.Send(resp); err != nil {
				log.Printf("failed to send response: %v", err)
				return
			}
		case <-stream.Context().Done():
			c.sendErr = errors.New("client canceled request")
			return
		}
	}
}

func (c *upstream) Close() {
	close(c.resp)
}

func newDownstream(stream ads.AggregatedDiscoveryService_StreamAggregatedResourcesClient) *downstream {
	req := make(chan *xds.DiscoveryRequest)
	resp := make(chan *xds.DiscoveryResponse)
	d := &downstream{
		req:  req,
		resp: resp,
	}
	// start a goroutine to drain the stream coming in, which will push
	go d.drain(stream, resp)
	go d.forward(stream, req)
	return d
}

// note this is nearly identical to upstream.drain, except for the type of the stream (which is a client rather than a server)
// and our channels
func (d *downstream) drain(stream ads.AggregatedDiscoveryService_StreamAggregatedResourcesClient, out chan<- *xds.DiscoveryResponse) {
	defer close(out)
	for {
		req, err := stream.Recv()
		if err != nil {
			if status.Code(err) == codes.Canceled || err == io.EOF {
				return
			}
			d.recErr = err
			return
		}
		out <- req
	}
}

// note this is nearly identical to upstream.forward, except for the type of the stream (which is a client rather than a server)
// and our channels
func (d *downstream) forward(stream ads.AggregatedDiscoveryService_StreamAggregatedResourcesClient, in <-chan *xds.DiscoveryRequest) {
	for {
		select {
		case resp, more := <-in:
			if !more || resp == nil {
				return
			}
			if err := stream.Send(resp); err != nil {
				log.Printf("failed to send response: %v", err)
				return
			}
		case <-stream.Context().Done():
			d.sendErr = errors.New("client canceled request")
			return
		}
	}
}

func (d *downstream) Close() {
	close(d.req)
}
