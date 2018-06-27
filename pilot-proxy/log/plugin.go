package log

import (
	"log"

	xds "github.com/envoyproxy/go-control-plane/envoy/api/v2"

	"github.com/tetratelabs/istio-tools/pilot-proxy"
)

type Plugin struct{}

var _ pilotproxy.Plugin = Plugin{}

func (Plugin) Name() string {
	return "log"
}

func (Plugin) Apply(response *xds.DiscoveryResponse) error {
	log.Printf("received response with payload: %v", response)
	return nil
}
