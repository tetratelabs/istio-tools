package log

import (
	"log"

	xds "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	xdscore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"

	"github.com/tetratelabs/istio-tools/pilot-proxy"
)

type Plugin struct{}

var _ pilotproxy.Plugin = Plugin{}

func (Plugin) Name() string {
	return "log"
}

func (Plugin) Apply(_ *xdscore.Node, response *xds.DiscoveryResponse) error {
	log.Printf("received response with payload: %v", response)
	return nil
}
