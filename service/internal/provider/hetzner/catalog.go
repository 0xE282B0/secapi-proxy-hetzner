package hetzner

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

type ComputeSKU struct {
	Name         string
	VCPU         int
	RAMGiB       int
	Architecture string
}

type CatalogImage struct {
	Name         string
	Type         string
	Architecture string
	Description  string
	Status       string
}

type preferredRegionContextKey struct{}

// WithPreferredRegion annotates the request context with a preferred region
// used to rank catalog SKUs by location availability.
func WithPreferredRegion(ctx context.Context, region string) context.Context {
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		return ctx
	}
	return context.WithValue(ctx, preferredRegionContextKey{}, region)
}

func (s *RegionService) ListComputeSKUs(ctx context.Context) ([]ComputeSKU, error) {
	if !s.configured {
		return nil, ErrNotConfigured
	}
	serverTypes, err := s.client.ServerType.All(ctx)
	if err != nil {
		return nil, err
	}

	preferredRegion := preferredRegionFromContext(ctx)
	ordered := make([]*hcloud.ServerType, 0, len(serverTypes))
	for _, st := range serverTypes {
		if st == nil {
			continue
		}
		ordered = append(ordered, st)
	}

	sort.Slice(ordered, func(i, j int) bool {
		a := ordered[i]
		b := ordered[j]
		aInRegion := serverTypeAvailableInRegion(a, preferredRegion)
		bInRegion := serverTypeAvailableInRegion(b, preferredRegion)
		if aInRegion != bInRegion {
			return aInRegion
		}
		aLocCount := serverTypeLocationCount(a)
		bLocCount := serverTypeLocationCount(b)
		if aLocCount != bLocCount {
			return aLocCount > bLocCount
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})

	skus := make([]ComputeSKU, 0, len(ordered))
	for _, st := range ordered {
		skus = append(skus, ComputeSKU{
			Name:         strings.ToLower(st.Name),
			VCPU:         st.Cores,
			RAMGiB:       int(st.Memory),
			Architecture: string(st.Architecture),
		})
	}

	return skus, nil
}

func (s *RegionService) GetComputeSKU(ctx context.Context, name string) (*ComputeSKU, error) {
	skus, err := s.ListComputeSKUs(ctx)
	if err != nil {
		return nil, err
	}
	for _, sku := range skus {
		if sku.Name == name {
			copySKU := sku
			return &copySKU, nil
		}
	}
	return nil, nil
}

func (s *RegionService) ListCatalogImages(ctx context.Context) ([]CatalogImage, error) {
	if !s.configured {
		return nil, ErrNotConfigured
	}
	images, err := s.client.Image.AllWithOpts(ctx, hcloud.ImageListOpts{IncludeDeprecated: true})
	if err != nil {
		return nil, err
	}

	out := make([]CatalogImage, 0, len(images))
	for _, image := range images {
		name := strings.ToLower(image.Name)
		if name == "" {
			name = "image-" + strings.ToLower(strings.ReplaceAll(string(image.Type), "_", "-")) + "-" + int64ToString(image.ID)
		}
		out = append(out, CatalogImage{
			Name:         name,
			Type:         string(image.Type),
			Architecture: string(image.Architecture),
			Description:  image.Description,
			Status:       string(image.Status),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Type < out[j].Type
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (s *RegionService) GetCatalogImage(ctx context.Context, name string) (*CatalogImage, error) {
	images, err := s.ListCatalogImages(ctx)
	if err != nil {
		return nil, err
	}
	for _, image := range images {
		if image.Name == name {
			copyImage := image
			return &copyImage, nil
		}
	}
	return nil, nil
}

func int64ToString(v int64) string {
	return strconv.FormatInt(v, 10)
}

func preferredRegionFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if region, ok := ctx.Value(preferredRegionContextKey{}).(string); ok {
		return strings.ToLower(strings.TrimSpace(region))
	}
	// Optional compatibility fallback for generic middleware that stores region
	// under a string key.
	if region, ok := ctx.Value("region").(string); ok {
		return strings.ToLower(strings.TrimSpace(region))
	}
	return ""
}

func serverTypeAvailableInRegion(st *hcloud.ServerType, region string) bool {
	if st == nil || region == "" {
		return true
	}
	region = strings.ToLower(strings.TrimSpace(region))

	for _, loc := range st.Locations {
		if loc.Location != nil && strings.EqualFold(loc.Location.Name, region) {
			return true
		}
	}
	for _, pricing := range st.Pricings {
		if pricing.Location != nil && strings.EqualFold(pricing.Location.Name, region) {
			return true
		}
	}
	return false
}

func serverTypeLocationCount(st *hcloud.ServerType) int {
	if st == nil {
		return 0
	}
	seen := map[string]struct{}{}
	for _, loc := range st.Locations {
		if loc.Location == nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(loc.Location.Name))
		if name != "" {
			seen[name] = struct{}{}
		}
	}
	for _, pricing := range st.Pricings {
		if pricing.Location == nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(pricing.Location.Name))
		if name != "" {
			seen[name] = struct{}{}
		}
	}
	return len(seen)
}
