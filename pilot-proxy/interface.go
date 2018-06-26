package pilotproxy

import (
	xds "github.com/envoyproxy/go-control-plane/envoy/api/v2"
)

type EventType string

const (
	// ClusterType is for updates to Envoy's clusters.
	ClusterType EventType = "type.googleapis.com/envoy.api.v2.Cluster"
	// EndpointType is for updates to Envoy's endpoint data in each cluster
	EndpointType EventType = "type.googleapis.com/envoy.api.v2.ClusterLoadAssignment"
	// ListenerType is for updates to Envoy's listeners
	ListenerType EventType = "type.googleapis.com/envoy.api.v2.Listener"
	// RouteType is for updates to Envoy's L7 routes
	RouteType EventType = "type.googleapis.com/envoy.api.v2.RouteConfiguration"
)

type Plugin interface {
	// Apply transformations to the response in-place, or return an error which will be forwarded to the client Envoy.
	//
	// Apply must be thread-safe: multiple goroutines will call Apply concurrently on the same Plugin instance.
	Apply(response *xds.DiscoveryResponse) error
}
