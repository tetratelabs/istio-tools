// Copyright 2018 Tetrate.io, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/golang/protobuf/proto"
	descriptor "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/spf13/cobra"
)

var tmpl = template.Must(template.New("grpc json transcoder filter").Parse(
	`# Created by github.com/tetratelabs/istio-tools/grpc-transcoder
apiVersion: networking.istio.io/v1alpha3
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
    insertPosition:
      index: BEFORE
      relativeTo: envoy.router
    filterName: envoy.grpc_json_transcoder
    filterType: HTTP
    filterConfig:
      services: {{ range .ProtoServices }} 
      - {{ . }}{{end}}
      protoDescriptorBin: {{ .DescriptorBinary }}
      printOptions:
        alwaysPrintPrimitiveFields: True
---
`))

// k8s CRDs only a megabyte of data; descriptors can be larger than this, and if they are they cannot be delivered.
const megabyte = 1000000

// getServices returns a list of matching services found in matching packages
func getServices(b *[]byte, packages []string, services []string) ([]string, error) {
	var (
		fds  descriptor.FileDescriptorSet
		out  []string
		rexp []*regexp.Regexp
		errs error
	)
	if err := proto.Unmarshal(*b, &fds); err != nil {
		return out, errors.Wrapf(err, "error proto unmarshall to FileDescriptorSet")
	}
	rexp = make([]*regexp.Regexp, 0)
	for _, r := range services {
		re, err := regexp.Compile(r)
		if err != nil {
			errs = multierror.Append(errs, err)
		} else {
			rexp = append(rexp, re)
		}
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
	return out, errs
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
		Short:   "gen-transcoder",
		Example: "gen-transcoder [--port 80] [--service foo] [--packages acme.example] [--services 'http.*,echo.*'] --descriptor /path/to/descriptor",
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
			// TODO: support outputting a file based CRD when descriptor is too large.
			if len(descriptorBytes) > megabyte {
				return fmt.Errorf("descriptor file is too large (%d bytes); CRDs cannot be larger than a megabyte", len(descriptorBytes))
			}

			protoServices, err = getServices(&descriptorBytes, packages, services)
			if err != nil {
				log.Printf("error extracting services from descriptor: %v\n", err)
				return err
			}
			sort.Strings(protoServices)

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
	cmd.PersistentFlags().StringVarP(&service, "service", "s", "grpc-transcoder",
		"The value of the `app` label for EnvoyFilter's workloadLabels config; see https://github.com/istio/api/blob/master/networking/v1alpha3/envoy_filter.proto#L59-L68")
	cmd.PersistentFlags().StringSliceVar(&packages, "packages", []string{},
		"Comma separated list of the proto package prefix names contained in the descriptor files. These must be fully qualified names, i.e. package_name.package_prefix")
	cmd.PersistentFlags().StringSliceVar(&services, "services", []string{},
		"Comma separated list of the proto service names contained in the descriptor files. These must be fully qualified names, i.e. package_name.service_name")
	cmd.PersistentFlags().StringVarP(&descriptorFilePath, "descriptor", "d", "", "Location of proto descriptor files relative to the server.")

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
