package main

import (
	"fmt"
	golog "log"
	"net"

	ads "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"

	"github.com/tetratelabs/istio-tools/pilot-proxy"
	"github.com/tetratelabs/istio-tools/pilot-proxy/log"
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
	var (
		downstream    string
		listenAddress string
	)

	cmd := &cobra.Command{
		Short:   "pilot-proxy",
		Example: "pilot-proxy --downstream istio-pilot.istio-system.svc.cluster.local:15001 --listen :9090",
		RunE: func(cmd *cobra.Command, args []string) error {
			// connect to the downstream and create a client for it
			conn, err := grpc.Dial(downstream, grpc.WithInsecure())
			if err != nil {
				return err
			}
			client := ads.NewAggregatedDiscoveryServiceClient(conn)

			// wire up our serving of the ADS API
			s := grpc.NewServer()
			ads.RegisterAggregatedDiscoveryServiceServer(s, pilotproxy.NewKissServer(client, plugins))

			// and serve it
			l, err := net.Listen("tcp", listenAddress)
			if err != nil {
				return fmt.Errorf("failed to listen on")
			}

			golog.Printf("starting ADS server listening on %q", listenAddress)
			return s.Serve(l)
		},
	}

	pflag.StringVarP(&downstream, "downstream", "d", "istio-pilot.istio-system.svc.cluster.local:15001",
		"The address, host:port, of the downstream ADS server we're proxying to.")
	pflag.StringVarP(&listenAddress, "listen", "l", ":29000",
		"The address this server will listen on; e.g. :29000. If no host is provided then the server will listen across of the host's IP addresses.")

	if err := cmd.Execute(); err != nil {
		golog.Fatal(err)
	}
}
