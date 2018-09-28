package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/golang/protobuf/proto"
	descriptor "github.com/golang/protobuf/protoc-gen-go/descriptor"

	"github.com/spf13/cobra"
)

var tmpl = template.Must(template.New("grpc json transcoder filter").Parse(`apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: {{ .ServiceName }}
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
---`))

// getServices returns a list of matching services found in matching packages
func getServices(b *[]byte, packages []string, services []string) ([]string, error) {
	var (
		fds  descriptor.FileDescriptorSet
		out  []string
		rexp []*regexp.Regexp
	)
	if err := proto.Unmarshal(*b, &fds); err != nil {
		log.Fatalf("error proto unmarshall to FileDescriptorSet, %v", err)
		return out, err
	}
	rexp = make([]*regexp.Regexp, 0)
	for _, r := range services {
		rexp = append(rexp, regexp.MustCompile(r))
	}

	// package
	findPkg := func(name string) bool {
		for _, p := range packages {
			if strings.HasPrefix(name, p) {
				return true
			}
		}
		return len(packages) == 0
	}

	// service
	findSvc := func(s string) bool {
		for _, r := range rexp {
			if r.MatchString(s) {
				return true
			}
		}
		return len(rexp) == 0
	}

	for _, f := range fds.GetFile() {
		if !findPkg(f.GetPackage()) {
			continue
		}
		for _, s := range f.GetService() {
			if findSvc(s.GetName()) {
				out = append(out, fmt.Sprintf("%s.%s", f.GetPackage(), s.GetName()))
			}
		}
	}
	return out, nil
}

func main() {
	var (
		service            string
		packages           []string
		services           []string
		protoServices      []string
		descriptorFilePath string
		port               int
	)

	cmd := &cobra.Command{
		Short:   "gen-envoyfilter",
		Example: "gen-envoyfilter [--port 80] [--service foo] [--packages acme.example] [--services 'http.*,echo.*'] --descriptor /path/to/descriptor",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat(descriptorFilePath); os.IsNotExist(err) {
				log.Printf("error opening descriptor file %q\n", descriptorFilePath)
				return err
			}

			descriptorBytes, err := ioutil.ReadFile(descriptorFilePath)
			if err != nil {
				log.Printf("error reading descriptor file %q\n", descriptorFilePath)
				return err
			}

			protoServices, err = getServices(&descriptorBytes, packages, services)
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
	if service == "" {
		service = "grpc-transcoder"
	}
	cmd.PersistentFlags().StringSliceVar(&packages, "packages", []string{},
		"Comma separated list of the proto package prefix names contained in the descriptor files. These must be fully qualified names, i.e. package_name.package_prefix")
	cmd.PersistentFlags().StringSliceVar(&services, "services", []string{},
		"Comma separated list of the proto service names contained in the descriptor files. These must be fully qualified names, i.e. package_name.service_name")
	cmd.PersistentFlags().StringVarP(&descriptorFilePath, "descriptor", "d", "", "Location of proto descriptor files relative to the server.")

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
