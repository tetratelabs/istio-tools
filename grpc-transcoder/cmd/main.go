package main

import (
	"log"

	"io/ioutil"

	"github.com/spf13/cobra"
	"github.com/tetratelabs/istio-tools/grpc-transcoder"
	"github.com/tetratelabs/istio-tools/pilot-proxy"
)

func main() {
	// TODO: flags to disable jaeger/prom
	var (
		downstream    string
		listenAddress string
		serviceName   string

		services           []string
		protoServices      []string
		descriptorFilePath string
	)

	cmd := &cobra.Command{
		Short:   "pilot-proxy",
		Example: "pilot-proxy --downstream istio-pilot.istio-system.svc.cluster.local:15001 --listen :9090",
		RunE: func(cmd *cobra.Command, args []string) error {
			descriptorBytes, err := ioutil.ReadFile(descriptorFilePath)
			if err != nil {
				return err
			}
			return pilotproxy.Serve(downstream, listenAddress, serviceName, map[string][]pilotproxy.Plugin{
				pilotproxy.ListenerType: {
					grpctranscoder.NewTranscoder(services, protoServices, descriptorBytes),
				},
			})
		},
	}

	cmd.PersistentFlags().StringVarP(&downstream, "downstream", "d", "istio-pilot.istio-system.svc.cluster.local:15001",
		"The address, host:port, of the downstream ADS server we're proxying to.")
	cmd.PersistentFlags().StringVarP(&listenAddress, "listen", "l", ":29000",
		"The address this server will listen on; e.g. :29000. If no host is provided then the server will listen across of the host's IP addresses.")
	cmd.PersistentFlags().StringVarP(&serviceName, "name", "n", "pilot-proxy", "The name of this service for tracing, metrics.")

	cmd.PersistentFlags().StringSliceVarP(&services, "services", "s", []string{},
		"Comma separated list of Istio services whose sidecars this proxy should insert gRPC transcoding filters into.")
	cmd.PersistentFlags().StringSliceVarP(&protoServices, "proto-services", "p", []string{},
		"Comma separated list of the proto service names contained in the descriptor files. These must be fully qualified names, i.e. package_name.service_name")
	cmd.PersistentFlags().StringVarP(&descriptorFilePath, "descriptor", "d", "", "Location of proto descriptor files relative to the server.")

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
