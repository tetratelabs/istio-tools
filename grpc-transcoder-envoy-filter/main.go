package main

import (
	"io/ioutil"
	"log"
	"os"
	"text/template"
	"encoding/base64"
	"bytes"

	xcoder "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/transcoder/v2"
	"istio.io/api/networking/v1alpha3"
	"github.com/spf13/cobra"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/ghodss/yaml"
	"github.com/gogo/protobuf/types"
	"fmt"
)



var _ = template.Must(template.New("grpc json transcoder filter").Parse(`
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: inventory-grpc-transcoder
spec:
  workloadLabels:
    app: {{ .ServiceName }}
  filters:
  - listenerMatch:
      portNumber: {{ .PortNumber }} 
      listenerType: SIDECAR_INBOUND
    # insert the transcoder filter before the HTTP router filter.
    insertPosition:
      index: BEFORE
      relativeTo: envoy.router
    filterName: envoy.grpc_json_transcoder
    filterType: HTTP
    filterConfig:
      services: {{ range .ProtoServices }} 
      - {{ . }}{{end}}
      protoDescriptorBin: !!binary |
        {{ .DescriptorBinary }}
`))

func main() {
	// TODO: flags to disable jaeger/prom
	var (
		service            string
		protoServices      []string
		descriptorFilePath string
		port               int
	)

	cmd := &cobra.Command{
		Short:   "gen-envoyfilter",
		Example: "gen-envoyfilter --port 80 --service inventory --proto-services foo.v1.Service,bar.v2.Service --descriptor ./path/to/descriptor",
		RunE: func(cmd *cobra.Command, args []string) error {
			descriptorBytes, err := ioutil.ReadFile(descriptorFilePath)
			if err != nil {
				return err
			}

			b := &bytes.Buffer{}
			b.Bytes()
			encoded := base64.StdEncoding.EncodeToString(descriptorBytes)

			filter := &xcoder.GrpcJsonTranscoder{
				DescriptorSet:             &xcoder.GrpcJsonTranscoder_ProtoDescriptorBin{
					ProtoDescriptorBin: []byte(encoded),
				},
				Services:                  protoServices,
				MatchIncomingRequestRoute: false,
			}
			tmp := &bytes.Buffer{}
			marshaller := &jsonpb.Marshaler{OrigName: true}
			if err := marshaller.Marshal(tmp, filter); err != nil {
				return err
			}

			s := &types.Struct{}
			if err := jsonpb.Unmarshal(tmp, s); err != nil {
				return nil
			}

			json, err :=  marshaller.MarshalToString(&v1alpha3.EnvoyFilter{
				WorkloadLabels: map[string]string{
					"app": service,
				},
				Filters: []*v1alpha3.EnvoyFilter_Filter{
					{
						ListenerMatch:  &v1alpha3.EnvoyFilter_ListenerMatch{
							PortNumber:       9080,
							ListenerType:     v1alpha3.EnvoyFilter_ListenerMatch_SIDECAR_INBOUND,
						},
						InsertPosition: &v1alpha3.EnvoyFilter_InsertPosition{
							Index:      v1alpha3.EnvoyFilter_InsertPosition_AFTER,
							RelativeTo: "envoy.router",
						},
						FilterType:     v1alpha3.EnvoyFilter_Filter_HTTP,
						FilterName:     "envoy.grpc_json_transcoder",
						FilterConfig:   s,
					},
				},
			})
			out, err := yaml.JSONToYAML([]byte(json))
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(os.Stdout, string(out))
			return err
		},
	}

	cmd.PersistentFlags().IntVarP(&port, "port", "p", 80, "Port that the HTTP/JSON -> gRPC transcoding filter should be attached to.")
	cmd.PersistentFlags().StringVarP(&service, "service", "s", "",
		"The value of the `app` label for EnvoyFilter's workloadLabels config; see https://github.com/istio/api/blob/master/networking/v1alpha3/envoy_filter.proto#L59-L68")
	cmd.PersistentFlags().StringSliceVar(&protoServices, "proto-services", []string{},
		"Comma separated list of the proto service names contained in the descriptor files. These must be fully qualified names, i.e. package_name.service_name")
	cmd.PersistentFlags().StringVarP(&descriptorFilePath, "descriptor", "d", "", "Location of proto descriptor files relative to the server.")

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
