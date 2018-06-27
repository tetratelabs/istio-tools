package main

import (
	golog "log"

	"github.com/spf13/cobra"

	"github.com/tetratelabs/istio-tools/pilot-proxy"
	"github.com/tetratelabs/istio-tools/pilot-proxy/log"
)

// Add new plugins here to wire them up at runtime.
var plugins = map[string][]pilotproxy.Plugin{
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

	cmd := &cobra.Command{
		Short:   "pilot-proxy",
		Example: "pilot-proxy --downstream istio-pilot.istio-system.svc.cluster.local:15001 --listen :9090",
		RunE: func(cmd *cobra.Command, args []string) error {
			return pilotproxy.Serve(downstream, listenAddress, serviceName, plugins)
		},
	}

	cmd.PersistentFlags().StringVarP(&downstream, "downstream", "d", "istio-pilot.istio-system.svc.cluster.local:15001",
		"The address, host:port, of the downstream ADS server we're proxying to.")
	cmd.PersistentFlags().StringVarP(&listenAddress, "listen", "l", ":29000",
		"The address this server will listen on; e.g. :29000. If no host is provided then the server will listen across of the host's IP addresses.")
	cmd.PersistentFlags().StringVarP(&serviceName, "name", "n", "pilot-proxy", "The name of this service for tracing, metrics.")

	if err := cmd.Execute(); err != nil {
		golog.Fatal(err)
	}
}
