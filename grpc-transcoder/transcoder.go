package grpctranscoder

import (
	"bytes"
	"errors"
	"fmt"

	xds "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	xdscore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	transcoder "github.com/envoyproxy/go-control-plane/envoy/config/filter/http/transcoder/v2"
	http_conn "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/tetratelabs/istio-tools/pilot-proxy"
)

const (
	// https://github.com/envoyproxy/envoy/blob/master/source/extensions/filters/http/well_known_names.h
	httpConnectionManagerName = "envoy.http_connection_manager"
	routerFilterName          = "envoy.router"
	transcoderFilterName      = "envoy.grpc_json_transcoder"
)

type Plugin struct {
	// the Envoy clusters we will insert transcoding config in to
	targetClusters map[string]bool
	filterConfig   *http_conn.HttpFilter
}

func NewTranscoder(targetClusters []string, services []string, descriptorBinary []byte) pilotproxy.Plugin {
	clusters := make(map[string]bool, len(targetClusters))
	for _, c := range targetClusters {
		clusters[c] = true
	}

	cfg := createConfig(services, descriptorBinary)
	s, err := messageToStruct(cfg)
	if err != nil {
		panic(fmt.Errorf("unexpected error marshalling static config: %v", err))
	}

	return &Plugin{
		targetClusters: clusters,
		filterConfig: &http_conn.HttpFilter{
			Name:   transcoderFilterName,
			Config: s,
		},
	}
}

func (p *Plugin) Name() string {
	return "grpctranscoder"
}

func (p *Plugin) Apply(node *xdscore.Node, response *xds.DiscoveryResponse) error {
	if !p.targetClusters[node.Cluster] || response.TypeUrl != pilotproxy.ListenerType {
		return nil
	}

	for i, resource := range response.Resources {
		var l *xds.Listener
		types.UnmarshalAny(&resource, l)

		// loop through each filter chain hunting for HTTP connection manager filters; we'll insert the transcoder
		// before the HTTP router filter there.
		for _, chain := range l.FilterChains {
			for j, f := range chain.Filters {
				if f.Name == httpConnectionManagerName {
					cfg := &http_conn.HttpConnectionManager{}
					if err := structToMessage(f.Config, cfg); err != nil {
						return fmt.Errorf("failed to unmarshal HTTP connection manager filter config with: %v", err)
					}
					routerIndex := 0
					for k, filter := range cfg.HttpFilters {
						if filter.Name == routerFilterName {
							routerIndex = k
						}
					}
					cfg.HttpFilters = insertAt(cfg.HttpFilters, routerIndex, p.filterConfig)

					updated, err := messageToStruct(cfg)
					if err != nil {
						return fmt.Errorf("failed to marshal HTTP connection manager filter config into a proto struct with: %v", err)
					}
					chain.Filters[j].Config = updated
				}
			}
		}

		updated, err := types.MarshalAny(l)
		if err != nil {
			return fmt.Errorf("failed to marshal updated listener into proto.Any with: %v", err)
		}
		response.Resources[i] = *updated
	}
	return nil
}

func createConfig(services []string, descriptorBinary []byte) *transcoder.GrpcJsonTranscoder {
	return &transcoder.GrpcJsonTranscoder{
		DescriptorSet: &transcoder.GrpcJsonTranscoder_ProtoDescriptorBin{
			ProtoDescriptorBin: descriptorBinary,
		},
		Services:                  services,
		MatchIncomingRequestRoute: false,
	}
}

func insertAt(src []*http_conn.HttpFilter, index int, filter *http_conn.HttpFilter) []*http_conn.HttpFilter {
	src = append(src, nil)
	copy(src[index+1:], src[index:])
	src[index] = filter
	return src
}

func messageToStruct(msg proto.Message) (*types.Struct, error) {
	if msg == nil {
		return nil, errors.New("nil message")
	}

	buf := &bytes.Buffer{}
	if err := (&jsonpb.Marshaler{OrigName: true}).Marshal(buf, msg); err != nil {
		return nil, err
	}

	pbs := &types.Struct{}
	if err := jsonpb.Unmarshal(buf, pbs); err != nil {
		return nil, err
	}

	return pbs, nil
}

func structToMessage(pbst *types.Struct, out proto.Message) error {
	if pbst == nil {
		return errors.New("nil struct")
	}

	buf := &bytes.Buffer{}
	if err := (&jsonpb.Marshaler{OrigName: true}).Marshal(buf, pbst); err != nil {
		return err
	}

	return jsonpb.Unmarshal(buf, out)
}
