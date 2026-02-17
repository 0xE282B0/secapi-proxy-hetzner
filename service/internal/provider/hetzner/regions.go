package hetzner

import (
	"context"
	"errors"
	"sort"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/config"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

var ErrNotConfigured = errors.New("hetzner api token not configured")

type Region struct {
	Name      string
	City      string
	Country   string
	Zones     []string
	Providers []Provider
}

type Provider struct {
	Name    string
	Version string
	URL     string
}

type RegionService struct {
	client      *hcloud.Client
	configured  bool
	publicBase  string
	cloudAPIURL string
	apiURL      string
}

func NewRegionService(cfg config.Config) *RegionService {
	configured := cfg.HetznerToken != ""
	client := hcloud.NewClient(
		hcloud.WithToken(cfg.HetznerToken),
		hcloud.WithEndpoint(cfg.HetznerCloudAPIURL),
		hcloud.WithHetznerEndpoint(cfg.HetznerPrimaryAPIURL),
	)
	return &RegionService{
		client:      client,
		configured:  configured,
		publicBase:  cfg.PublicBaseURL,
		cloudAPIURL: cfg.HetznerCloudAPIURL,
		apiURL:      cfg.HetznerPrimaryAPIURL,
	}
}

func (s *RegionService) ListRegions(ctx context.Context) ([]Region, error) {
	if !s.configured {
		return nil, ErrNotConfigured
	}

	locations, err := s.client.Location.All(ctx)
	if err != nil {
		return nil, err
	}
	dataCenters, err := s.client.Datacenter.All(ctx)
	if err != nil {
		return nil, err
	}

	zonesByLocation := make(map[string][]string)
	for _, dc := range dataCenters {
		if dc.Location == nil {
			continue
		}
		zonesByLocation[dc.Location.Name] = append(zonesByLocation[dc.Location.Name], dc.Name)
	}

	regions := make([]Region, 0, len(locations))
	for _, loc := range locations {
		zones := dedupeSorted(zonesByLocation[loc.Name])
		regions = append(regions, Region{
			Name:    loc.Name,
			City:    loc.City,
			Country: loc.Country,
			Zones:   zones,
			Providers: []Provider{
				{Name: "hetzner.cloud", Version: "v1", URL: s.cloudAPIURL},
				{Name: "hetzner", Version: "v1", URL: s.apiURL},
				{Name: "seca.region", Version: "v1", URL: s.publicBase},
			},
		})
	}

	sort.Slice(regions, func(i, j int) bool {
		return regions[i].Name < regions[j].Name
	})
	return regions, nil
}

func (s *RegionService) GetRegion(ctx context.Context, name string) (*Region, error) {
	regions, err := s.ListRegions(ctx)
	if err != nil {
		return nil, err
	}
	for _, region := range regions {
		if region.Name == name {
			copyRegion := region
			return &copyRegion, nil
		}
	}
	return nil, nil
}

func dedupeSorted(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
