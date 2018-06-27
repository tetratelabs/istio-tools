package pilotproxy

import (
	xds "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	xdscore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
)

const (
	// ClusterType is for updates to Envoy's clusters.
	ClusterType = "type.googleapis.com/envoy.api.v2.Cluster"
	// EndpointType is for updates to Envoy's endpoint data in each cluster
	EndpointType = "type.googleapis.com/envoy.api.v2.ClusterLoadAssignment"
	// ListenerType is for updates to Envoy's listeners
	ListenerType = "type.googleapis.com/envoy.api.v2.Listener"
	// RouteType is for updates to Envoy's L7 routes
	RouteType = "type.googleapis.com/envoy.api.v2.RouteConfiguration"
)

type Plugin interface {
	// Name of this plugin. Used for logging and metrics.
	Name() string
	// Apply transformations to the response in-place, or return an error which will be forwarded to the client Envoy.
	//
	// Apply must be thread-safe: multiple goroutines will call Apply concurrently on the same Plugin instance.
	Apply(node *xdscore.Node, response *xds.DiscoveryResponse) error
}
