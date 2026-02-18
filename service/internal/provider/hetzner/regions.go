package hetzner

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

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
	client          *hcloud.Client
	configured      bool
	publicBase      string
	cloudAPIURL     string
	apiURL          string
	availCacheTTL   time.Duration
	conformanceMode bool

	serverTypesCacheMu sync.RWMutex
	serverTypesCacheAt time.Time
	serverTypesCache   []*hcloud.ServerType
}

func NewRegionService(cfg config.Config) *RegionService {
	configured := cfg.HetznerToken != ""
	client := hcloud.NewClient(
		hcloud.WithToken(cfg.HetznerToken),
		hcloud.WithEndpoint(cfg.HetznerCloudAPIURL),
		hcloud.WithHetznerEndpoint(cfg.HetznerPrimaryAPIURL),
	)
	return &RegionService{
		client:          client,
		configured:      configured,
		publicBase:      cfg.PublicBaseURL,
		cloudAPIURL:     cfg.HetznerCloudAPIURL,
		apiURL:          cfg.HetznerPrimaryAPIURL,
		availCacheTTL:   cfg.HetznerAvailCacheTTL,
		conformanceMode: cfg.ConformanceMode,
	}
}

func (s *RegionService) listServerTypes(ctx context.Context) ([]*hcloud.ServerType, error) {
	if s.availCacheTTL <= 0 {
		return s.client.ServerType.All(ctx)
	}

	now := time.Now()
	s.serverTypesCacheMu.RLock()
	if len(s.serverTypesCache) > 0 && now.Sub(s.serverTypesCacheAt) < s.availCacheTTL {
		cached := cloneServerTypes(s.serverTypesCache)
		s.serverTypesCacheMu.RUnlock()
		return cached, nil
	}
	s.serverTypesCacheMu.RUnlock()

	serverTypes, err := s.client.ServerType.All(ctx)
	if err != nil {
		return nil, err
	}
	cloned := cloneServerTypes(serverTypes)

	s.serverTypesCacheMu.Lock()
	s.serverTypesCache = cloneServerTypes(cloned)
	s.serverTypesCacheAt = time.Now()
	s.serverTypesCacheMu.Unlock()

	return cloned, nil
}

func cloneServerTypes(in []*hcloud.ServerType) []*hcloud.ServerType {
	if len(in) == 0 {
		return nil
	}
	out := make([]*hcloud.ServerType, len(in))
	copy(out, in)
	return out
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
				{Name: "seca.workspace", Version: "v1", URL: s.publicBase + "/workspace"},
				{Name: "seca.compute", Version: "v1", URL: s.publicBase + "/compute"},
				{Name: "seca.storage", Version: "v1", URL: s.publicBase + "/storage"},
				{Name: "seca.network", Version: "v1", URL: s.publicBase + "/network"},
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
