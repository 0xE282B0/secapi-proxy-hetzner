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

func (s *RegionService) ListComputeSKUs(ctx context.Context) ([]ComputeSKU, error) {
	if !s.configured {
		return nil, ErrNotConfigured
	}
	serverTypes, err := s.client.ServerType.All(ctx)
	if err != nil {
		return nil, err
	}

	skus := make([]ComputeSKU, 0, len(serverTypes))
	for _, st := range serverTypes {
		r := int(st.Memory)
		skus = append(skus, ComputeSKU{
			Name:         strings.ToLower(st.Name),
			VCPU:         st.Cores,
			RAMGiB:       r,
			Architecture: string(st.Architecture),
		})
	}

	sort.Slice(skus, func(i, j int) bool {
		return skus[i].Name < skus[j].Name
	})
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
