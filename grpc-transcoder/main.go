package main

import (
	"encoding/base64"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"text/template"

	"github.com/golang/protobuf/proto"
	descriptor "github.com/golang/protobuf/protoc-gen-go/descriptor"

	"github.com/spf13/cobra"
)

var tmpl = template.Must(template.New("grpc json transcoder filter").Parse(`
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: {{ .ServiceName }}-grpc-transcoder
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
      protoDescriptorBin: !!binary |
        {{ .DescriptorBinary }}
      services: {{ range .ProtoServices }} 
      - {{ . }}{{end}}
`))

// getTetrateServices looks for "tetrate.xxx" package, and returns a list
// of services found in each such package
func getTetrateServices(b *[]byte) ([]string, error) {
	var (
		fds descriptor.FileDescriptorSet
		svc []string
	)

	if err := proto.Unmarshal(*b, &fds); err != nil {
		log.Fatalf("proto unmarshall to FileDescriptorSet error: %v", err)
		return svc, err
	}
	for _, f := range fds.GetFile() {
		m, _ := regexp.MatchString("tetrate.", f.GetPackage())
		if m == true {
			for _, s := range f.GetService() {
				svc = append(svc, f.GetPackage()+"."+s.GetName())
			}
		}
	}
	return svc, nil
}

func main() {
	var (
		service            string
		protoServices      []string
		descriptorFilePath string
		port               int
	)

	cmd := &cobra.Command{
		Short:   "gen-envoyfilter",
		Example: "gen-envoyfilter --port 80 --service foo --descriptor ./path/to/descriptor",
		RunE: func(cmd *cobra.Command, args []string) error {
			descriptorBytes, err := ioutil.ReadFile(descriptorFilePath)
			if err != nil {
				return err
			}

			protoServices, err = getTetrateServices(&descriptorBytes)
			if err != nil {
				return err
			}

			encoded := base64.StdEncoding.EncodeToString(descriptorBytes)
			params := map[string]interface{}{
				"ServiceName":      service,
				"PortNumber":       port,
				"DescriptorBinary": encoded,
				"ProtoServices":    protoServices,
			}
			return tmpl.Execute(os.Stdout, params)
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
