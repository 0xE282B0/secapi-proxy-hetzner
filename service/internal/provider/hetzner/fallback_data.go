package hetzner

import (
	_ "embed"
	"encoding/json"
)

type fallbackRegionRecord struct {
	Name    string   `json:"name"`
	City    string   `json:"city"`
	Country string   `json:"country"`
	Zones   []string `json:"zones"`
}

var (
	//go:embed fallback/regions.json
	fallbackRegionsJSON []byte

	//go:embed fallback/compute_skus.json
	fallbackComputeSKUsJSON []byte

	//go:embed fallback/catalog_images.json
	fallbackCatalogImagesJSON []byte
)

var fallbackRegionsData []fallbackRegionRecord
var fallbackComputeSKUsData []ComputeSKU
var fallbackCatalogImagesData []CatalogImage

func init() {
	if err := json.Unmarshal(fallbackRegionsJSON, &fallbackRegionsData); err != nil {
		panic("invalid embedded fallback regions json: " + err.Error())
	}
	if err := json.Unmarshal(fallbackComputeSKUsJSON, &fallbackComputeSKUsData); err != nil {
		panic("invalid embedded fallback compute skus json: " + err.Error())
	}
	if err := json.Unmarshal(fallbackCatalogImagesJSON, &fallbackCatalogImagesData); err != nil {
		panic("invalid embedded fallback catalog images json: " + err.Error())
	}
}

func loadFallbackRegions() []fallbackRegionRecord {
	out := make([]fallbackRegionRecord, len(fallbackRegionsData))
	copy(out, fallbackRegionsData)
	return out
}

func loadFallbackComputeSKUs() []ComputeSKU {
	out := make([]ComputeSKU, len(fallbackComputeSKUsData))
	copy(out, fallbackComputeSKUsData)
	return out
}

func loadFallbackCatalogImages() []CatalogImage {
	out := make([]CatalogImage, len(fallbackCatalogImagesData))
	copy(out, fallbackCatalogImagesData)
	return out
}
