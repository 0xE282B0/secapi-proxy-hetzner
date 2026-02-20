package hetzner

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

type Network struct {
	Name      string
	CIDR      string
	Labels    map[string]string
	CreatedAt time.Time
}

type NetworkCreateRequest struct {
	Name   string
	CIDR   string
	Labels map[string]string
}

func (s *RegionService) ListNetworks(ctx context.Context) ([]Network, error) {
	if !s.configured {
		return nil, ErrNotConfigured
	}
	items, err := s.clientFor(ctx).Network.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Network, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		out = append(out, networkFromHCloud(item))
	}
	return out, nil
}

func (s *RegionService) GetNetwork(ctx context.Context, name string) (*Network, error) {
	if !s.configured {
		return nil, ErrNotConfigured
	}
	item, _, err := s.clientFor(ctx).Network.GetByName(ctx, strings.TrimSpace(name))
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	network := networkFromHCloud(item)
	return &network, nil
}

func (s *RegionService) CreateOrUpdateNetwork(ctx context.Context, req NetworkCreateRequest) (*Network, bool, error) {
	if !s.configured {
		return nil, false, ErrNotConfigured
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, false, invalidRequestError("network name is required")
	}
	cidr := strings.TrimSpace(req.CIDR)
	if cidr == "" {
		return nil, false, invalidRequestError("network cidr is required")
	}
	_, ipRange, err := net.ParseCIDR(cidr)
	if err != nil || ipRange == nil {
		return nil, false, invalidRequestError("invalid network cidr")
	}

	existing, _, err := s.clientFor(ctx).Network.GetByName(ctx, name)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		updated, _, updateErr := s.clientFor(ctx).Network.Update(ctx, existing, hcloud.NetworkUpdateOpts{
			Labels: req.Labels,
		})
		if updateErr != nil {
			return nil, false, updateErr
		}
		network := networkFromHCloud(updated)
		return &network, false, nil
	}

	created, _, err := s.clientFor(ctx).Network.Create(ctx, hcloud.NetworkCreateOpts{
		Name:    name,
		IPRange: ipRange,
		Labels:  req.Labels,
	})
	if err != nil {
		return nil, false, err
	}
	if created == nil {
		return nil, false, fmt.Errorf("hetzner returned empty network")
	}
	network := networkFromHCloud(created)
	return &network, true, nil
}

func (s *RegionService) DeleteNetwork(ctx context.Context, name string) (bool, error) {
	if !s.configured {
		return false, ErrNotConfigured
	}
	item, _, err := s.clientFor(ctx).Network.GetByName(ctx, strings.TrimSpace(name))
	if err != nil {
		return false, err
	}
	if item == nil {
		return false, nil
	}
	_, err = s.clientFor(ctx).Network.Delete(ctx, item)
	if err != nil {
		return false, err
	}
	return true, nil
}

func networkFromHCloud(item *hcloud.Network) Network {
	cidr := ""
	if item.IPRange != nil {
		cidr = item.IPRange.String()
	}
	return Network{
		Name:      strings.ToLower(strings.TrimSpace(item.Name)),
		CIDR:      cidr,
		Labels:    item.Labels,
		CreatedAt: item.Created,
	}
}

