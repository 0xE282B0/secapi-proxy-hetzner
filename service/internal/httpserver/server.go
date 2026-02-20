package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/config"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/provider/hetzner"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

type RegionProvider interface {
	ListRegions(ctx context.Context) ([]hetzner.Region, error)
	GetRegion(ctx context.Context, name string) (*hetzner.Region, error)
}

type CatalogProvider interface {
	ListComputeSKUs(ctx context.Context) ([]hetzner.ComputeSKU, error)
	GetComputeSKU(ctx context.Context, name string) (*hetzner.ComputeSKU, error)
	ListCatalogImages(ctx context.Context) ([]hetzner.CatalogImage, error)
	GetCatalogImage(ctx context.Context, name string) (*hetzner.CatalogImage, error)
}

type ComputeStorageProvider interface {
	ListInstances(ctx context.Context) ([]hetzner.Instance, error)
	GetInstance(ctx context.Context, name string) (*hetzner.Instance, error)
	CreateOrUpdateInstance(ctx context.Context, req hetzner.InstanceCreateRequest) (*hetzner.Instance, bool, string, error)
	DeleteInstance(ctx context.Context, name string) (bool, string, error)
	StartInstance(ctx context.Context, name string) (bool, string, error)
	StopInstance(ctx context.Context, name string) (bool, string, error)
	RestartInstance(ctx context.Context, name string) (bool, string, error)

	ListBlockStorages(ctx context.Context) ([]hetzner.BlockStorage, error)
	GetBlockStorage(ctx context.Context, name string) (*hetzner.BlockStorage, error)
	CreateOrUpdateBlockStorage(ctx context.Context, req hetzner.BlockStorageCreateRequest) (*hetzner.BlockStorage, bool, string, error)
	DeleteBlockStorage(ctx context.Context, name string) (bool, error)
	AttachBlockStorage(ctx context.Context, name, instanceName string) (bool, string, error)
	DetachBlockStorage(ctx context.Context, name string) (bool, string, error)
}

type NetworkProvider interface {
	ListNetworks(ctx context.Context) ([]hetzner.Network, error)
	GetNetwork(ctx context.Context, name string) (*hetzner.Network, error)
	CreateOrUpdateNetwork(ctx context.Context, req hetzner.NetworkCreateRequest) (*hetzner.Network, bool, error)
	DeleteNetwork(ctx context.Context, name string) (bool, error)

	ListSecurityGroups(ctx context.Context) ([]hetzner.SecurityGroup, error)
	GetSecurityGroup(ctx context.Context, name string) (*hetzner.SecurityGroup, error)
	CreateOrUpdateSecurityGroup(ctx context.Context, req hetzner.SecurityGroupCreateRequest) (*hetzner.SecurityGroup, bool, error)
	DeleteSecurityGroup(ctx context.Context, name string) (bool, error)
}

type statusResponse struct {
	Status string `json:"status"`
}

type problemResponse struct {
	Type     string          `json:"type"`
	Title    string          `json:"title"`
	Status   int             `json:"status"`
	Detail   string          `json:"detail"`
	Instance string          `json:"instance"`
	Sources  []problemSource `json:"sources"`
}

type problemSource struct {
	Pointer   string `json:"pointer"`
	Parameter string `json:"parameter"`
}

type responseMetaObject struct {
	Provider string `json:"provider"`
	Resource string `json:"resource"`
	Verb     string `json:"verb"`
}

type resourceMetadata struct {
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	Resource        string `json:"resource"`
	Verb            string `json:"verb"`
	CreatedAt       string `json:"createdAt,omitempty"`
	LastModifiedAt  string `json:"lastModifiedAt,omitempty"`
	ResourceVersion int64  `json:"resourceVersion,omitempty"`
	APIVersion      string `json:"apiVersion"`
	Kind            string `json:"kind"`
	Ref             string `json:"ref"`
	Tenant          string `json:"tenant,omitempty"`
	Workspace       string `json:"workspace,omitempty"`
	Network         string `json:"network,omitempty"`
	Region          string `json:"region,omitempty"`
}

type regionIterator struct {
	Items    []regionResource   `json:"items"`
	Metadata responseMetaObject `json:"metadata"`
}

type regionResource struct {
	Metadata resourceMetadata `json:"metadata"`
	Spec     regionSpec       `json:"spec"`
}

type regionSpec struct {
	AvailableZones []string           `json:"availableZones"`
	Providers      []regionSpecVendor `json:"providers"`
}

type regionSpecVendor struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	URL     string `json:"url"`
}

type computeSKUIterator struct {
	Items    []computeSKUResource `json:"items"`
	Metadata responseMetaObject   `json:"metadata"`
}

type computeSKUResource struct {
	Metadata resourceMetadata `json:"metadata"`
	Spec     computeSKUSpec   `json:"spec"`
}

type computeSKUSpec struct {
	VCPU int `json:"vCPU"`
	RAM  int `json:"ram"`
}

type imageIterator struct {
	Items    []imageResource    `json:"items"`
	Metadata responseMetaObject `json:"metadata"`
}

type imageResource struct {
	Metadata resourceMetadata  `json:"metadata"`
	Labels   map[string]string `json:"labels,omitempty"`
	Spec     imageSpec         `json:"spec"`
	Status   imageStatus       `json:"status"`
}

type imageSpec struct {
	BlockStorageRef refObject `json:"blockStorageRef"`
	CPUArchitecture string    `json:"cpuArchitecture"`
}

type imageStatus struct {
	State string `json:"state"`
}

type refObject struct {
	Resource string `json:"resource"`
}

func (r refObject) MarshalJSON() ([]byte, error) {
	// Conformance expects references serialized as the compact string form.
	return json.Marshal(r.Resource)
}

func (r *refObject) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		r.Resource = ""
		return nil
	}

	// Accept union form as a plain reference string, e.g. "skus/cx23".
	if strings.HasPrefix(raw, "\"") {
		var ref string
		if err := json.Unmarshal(data, &ref); err != nil {
			return err
		}
		r.Resource = ref
		return nil
	}

	// Accept object form, e.g. {"resource":"skus/cx23"}.
	var obj struct {
		Resource string `json:"resource"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("invalid reference payload: %w", err)
	}
	r.Resource = obj.Resource
	return nil
}

type wellknownResponse struct {
	Version   string              `json:"version"`
	Endpoints []wellknownEndpoint `json:"endpoints"`
}

type wellknownEndpoint struct {
	Provider string `json:"provider"`
	URL      string `json:"url"`
}

type Servers struct {
	Public *http.Server
	Admin  *http.Server
}

func New(
	cfg config.Config,
	store *state.Store,
	regionProvider RegionProvider,
	catalogProvider CatalogProvider,
	computeStorageProvider ComputeStorageProvider,
	networkProvider NetworkProvider,
) Servers {
	publicMux := http.NewServeMux()
	publicMux.HandleFunc("/healthz", healthz)
	publicMux.HandleFunc("/readyz", readyz(store))
	publicMux.HandleFunc("/.wellknown/secapi", wellknown(cfg))
	publicMux.HandleFunc("/v1/regions", listRegions(regionProvider))
	publicMux.HandleFunc("/v1/regions/{name}", getRegion(regionProvider))
	publicMux.HandleFunc("/v1/tenants/{tenant}/roles", listRoles(store))
	publicMux.HandleFunc("/v1/tenants/{tenant}/roles/{name}", roleCRUD(store))
	publicMux.HandleFunc("/v1/tenants/{tenant}/role-assignments", listRoleAssignments(store))
	publicMux.HandleFunc("/v1/tenants/{tenant}/role-assignments/{name}", roleAssignmentCRUD(store))
	publicMux.HandleFunc("/workspace/v1/tenants/{tenant}/workspaces", listWorkspaces(store))
	publicMux.HandleFunc("/workspace/v1/tenants/{tenant}/workspaces/{name}", workspaceCRUD(store))
	publicMux.HandleFunc("/compute/v1/tenants/{tenant}/skus", listComputeSKUs(catalogProvider))
	publicMux.HandleFunc("/compute/v1/tenants/{tenant}/skus/{name}", getComputeSKU(catalogProvider))
	publicMux.HandleFunc("/storage/v1/tenants/{tenant}/skus", listStorageSKUs())
	publicMux.HandleFunc("/storage/v1/tenants/{tenant}/skus/{name}", getStorageSKU())
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/skus", listNetworkSKUs())
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/skus/{name}", getNetworkSKU())
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/workspaces/{workspace}/networks", listNetworksProvider(networkProvider, store))
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/workspaces/{workspace}/networks/{name}", networkCRUDProvider(networkProvider, store))
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/workspaces/{workspace}/networks/{network}/route-tables", listRouteTables(store))
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/workspaces/{workspace}/networks/{network}/route-tables/{name}", routeTableCRUD(store))
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/workspaces/{workspace}/networks/{network}/subnets", listSubnets(store))
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/workspaces/{workspace}/networks/{network}/subnets/{name}", subnetCRUD(store))
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/workspaces/{workspace}/nics", listNICs(store))
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/workspaces/{workspace}/nics/{name}", nicCRUD(store))
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/workspaces/{workspace}/public-ips", listPublicIPs(store))
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/workspaces/{workspace}/public-ips/{name}", publicIPCRUD(store))
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/workspaces/{workspace}/security-groups", listSecurityGroups(networkProvider, store))
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/workspaces/{workspace}/security-groups/{name}", securityGroupCRUD(networkProvider, store))
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/workspaces/{workspace}/internet-gateways", listInternetGateways(store))
	publicMux.HandleFunc("/network/v1/tenants/{tenant}/workspaces/{workspace}/internet-gateways/{name}", internetGatewayCRUD(store, computeStorageProvider, cfg))
	publicMux.HandleFunc("/storage/v1/tenants/{tenant}/images", listImages(catalogProvider))
	publicMux.HandleFunc("/storage/v1/tenants/{tenant}/images/{name}", imageCRUD(catalogProvider, cfg.ConformanceMode))
	publicMux.HandleFunc("/compute/v1/tenants/{tenant}/workspaces/{workspace}/instances", listInstances(computeStorageProvider, store))
	publicMux.HandleFunc("/compute/v1/tenants/{tenant}/workspaces/{workspace}/instances/{name}", instanceCRUD(computeStorageProvider, store))
	publicMux.HandleFunc("/compute/v1/tenants/{tenant}/workspaces/{workspace}/instances/{name}/start", startInstance(computeStorageProvider, store))
	publicMux.HandleFunc("/compute/v1/tenants/{tenant}/workspaces/{workspace}/instances/{name}/stop", stopInstance(computeStorageProvider, store))
	publicMux.HandleFunc("/compute/v1/tenants/{tenant}/workspaces/{workspace}/instances/{name}/restart", restartInstance(computeStorageProvider, store))
	publicMux.HandleFunc("/storage/v1/tenants/{tenant}/workspaces/{workspace}/block-storages", listBlockStorages(computeStorageProvider, store))
	publicMux.HandleFunc("/storage/v1/tenants/{tenant}/workspaces/{workspace}/block-storages/{name}", blockStorageCRUD(computeStorageProvider, store))
	publicMux.HandleFunc("/storage/v1/tenants/{tenant}/workspaces/{workspace}/block-storages/{name}/attach", attachBlockStorage(computeStorageProvider, store))
	publicMux.HandleFunc("/storage/v1/tenants/{tenant}/workspaces/{workspace}/block-storages/{name}/detach", detachBlockStorage(computeStorageProvider, store))

	adminMux := http.NewServeMux()
	adminMux.HandleFunc(
		"/admin/v1/tenants/{tenant}/workspaces/{workspace}/providers/hetzner",
		requireAdminAuth(cfg.AdminToken, adminWorkspaceHetznerBinding(store, regionProvider)),
	)

	return Servers{
		Public: &http.Server{
			Addr:              cfg.ListenAddr,
			Handler:           publicMux,
			ReadHeaderTimeout: 10 * time.Second,
		},
		Admin: &http.Server{
			Addr:              cfg.AdminListenAddr,
			Handler:           adminMux,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

func readyz(store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := store.Ping(r.Context()); err != nil {
			respondJSON(w, http.StatusServiceUnavailable, statusResponse{Status: "db_unavailable"})
			return
		}
		respondJSON(w, http.StatusOK, statusResponse{Status: "ready"})
	}
}

func wellknown(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		base := strings.TrimRight(cfg.PublicBaseURL, "/")
		respondJSON(w, http.StatusOK, wellknownResponse{
			Version: "v1",
			Endpoints: []wellknownEndpoint{
				{Provider: "seca.region/v1", URL: base + "/v1"},
				{Provider: "seca.compute/v1", URL: base + "/compute/v1"},
				{Provider: "seca.storage/v1", URL: base + "/storage/v1"},
			},
		})
	}
}

func listRegions(regionProvider RegionProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		regions, err := regionProvider.ListRegions(r.Context())
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		items := make([]regionResource, 0, len(regions))
		for _, region := range regions {
			items = append(items, toRegionResource(region, now, http.MethodGet))
		}
		respondJSON(w, http.StatusOK, regionIterator{Items: items, Metadata: responseMetaObject{Provider: "seca.region/v1", Resource: "regions", Verb: http.MethodGet}})
	}
}

func getRegion(regionProvider RegionProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		name := r.PathValue("name")
		if name == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "region name is required", r.URL.Path)
			return
		}
		region, err := regionProvider.GetRegion(r.Context(), name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if region == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "region not found", r.URL.Path)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		respondJSON(w, http.StatusOK, toRegionResource(*region, now, http.MethodGet))
	}
}

func listComputeSKUs(catalogProvider CatalogProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		tenant := r.PathValue("tenant")
		if tenant == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant is required", r.URL.Path)
			return
		}
		skus, err := catalogProvider.ListComputeSKUs(r.Context())
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		items := make([]computeSKUResource, 0, len(skus))
		for _, sku := range skus {
			items = append(items, computeSKUResource{Metadata: resourceMetadata{Name: sku.Name, Provider: "seca.compute/v1", Resource: "tenants/" + tenant + "/skus/" + sku.Name, Verb: http.MethodGet, CreatedAt: now, LastModifiedAt: now, ResourceVersion: 1, APIVersion: "v1", Kind: "instance-sku", Ref: "seca.compute/v1/tenants/" + tenant + "/skus/" + sku.Name, Tenant: tenant, Region: "global"}, Spec: computeSKUSpec{VCPU: sku.VCPU, RAM: sku.RAMGiB}})
		}
		respondJSON(w, http.StatusOK, computeSKUIterator{Items: items, Metadata: responseMetaObject{Provider: "seca.compute/v1", Resource: "tenants/" + tenant + "/skus", Verb: http.MethodGet}})
	}
}

func getComputeSKU(catalogProvider CatalogProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		tenant := r.PathValue("tenant")
		name := strings.ToLower(r.PathValue("name"))
		if tenant == "" || name == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and sku name are required", r.URL.Path)
			return
		}
		sku, err := catalogProvider.GetComputeSKU(r.Context(), name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if sku == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "compute sku not found", r.URL.Path)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		respondJSON(w, http.StatusOK, computeSKUResource{Metadata: resourceMetadata{Name: sku.Name, Provider: "seca.compute/v1", Resource: "tenants/" + tenant + "/skus/" + sku.Name, Verb: http.MethodGet, CreatedAt: now, LastModifiedAt: now, ResourceVersion: 1, APIVersion: "v1", Kind: "instance-sku", Ref: "seca.compute/v1/tenants/" + tenant + "/skus/" + sku.Name, Tenant: tenant, Region: "global"}, Spec: computeSKUSpec{VCPU: sku.VCPU, RAM: sku.RAMGiB}})
	}
}

func listStorageSKUs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		tenant := r.PathValue("tenant")
		if tenant == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant is required", r.URL.Path)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		items := []computeSKUResource{
			{
				Metadata: resourceMetadata{
					Name:            "hcloud-volume",
					Provider:        "seca.storage/v1",
					Resource:        "tenants/" + tenant + "/skus/hcloud-volume",
					Verb:            http.MethodGet,
					CreatedAt:       now,
					LastModifiedAt:  now,
					ResourceVersion: 1,
					APIVersion:      "v1",
					Kind:            "storage-sku",
					Ref:             "seca.storage/v1/tenants/" + tenant + "/skus/hcloud-volume",
					Tenant:          tenant,
					Region:          "global",
				},
				Spec: computeSKUSpec{VCPU: 0, RAM: 0},
			},
		}
		respondJSON(w, http.StatusOK, computeSKUIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.storage/v1", Resource: "tenants/" + tenant + "/skus", Verb: http.MethodGet},
		})
	}
}

func getStorageSKU() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		tenant := r.PathValue("tenant")
		name := strings.ToLower(r.PathValue("name"))
		if tenant == "" || name == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and sku name are required", r.URL.Path)
			return
		}
		if name != "hcloud-volume" {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "storage sku not found", r.URL.Path)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		respondJSON(w, http.StatusOK, computeSKUResource{
			Metadata: resourceMetadata{
				Name:            "hcloud-volume",
				Provider:        "seca.storage/v1",
				Resource:        "tenants/" + tenant + "/skus/hcloud-volume",
				Verb:            http.MethodGet,
				CreatedAt:       now,
				LastModifiedAt:  now,
				ResourceVersion: 1,
				APIVersion:      "v1",
				Kind:            "storage-sku",
				Ref:             "seca.storage/v1/tenants/" + tenant + "/skus/hcloud-volume",
				Tenant:          tenant,
				Region:          "global",
			},
			Spec: computeSKUSpec{VCPU: 0, RAM: 0},
		})
	}
}

func listNetworkSKUs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		tenant := r.PathValue("tenant")
		if tenant == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant is required", r.URL.Path)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		items := []computeSKUResource{
			{
				Metadata: resourceMetadata{
					Name:            "hcloud-network",
					Provider:        "seca.network/v1",
					Resource:        "tenants/" + tenant + "/skus/hcloud-network",
					Verb:            http.MethodGet,
					CreatedAt:       now,
					LastModifiedAt:  now,
					ResourceVersion: 1,
					APIVersion:      "v1",
					Kind:            "network-sku",
					Ref:             "seca.network/v1/tenants/" + tenant + "/skus/hcloud-network",
					Tenant:          tenant,
					Region:          "global",
				},
				Spec: computeSKUSpec{VCPU: 0, RAM: 0},
			},
		}
		respondJSON(w, http.StatusOK, computeSKUIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.network/v1", Resource: "tenants/" + tenant + "/skus", Verb: http.MethodGet},
		})
	}
}

func getNetworkSKU() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		tenant := r.PathValue("tenant")
		name := strings.ToLower(r.PathValue("name"))
		if tenant == "" || name == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and sku name are required", r.URL.Path)
			return
		}
		if name != "hcloud-network" {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "network sku not found", r.URL.Path)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		respondJSON(w, http.StatusOK, computeSKUResource{
			Metadata: resourceMetadata{
				Name:            "hcloud-network",
				Provider:        "seca.network/v1",
				Resource:        "tenants/" + tenant + "/skus/hcloud-network",
				Verb:            http.MethodGet,
				CreatedAt:       now,
				LastModifiedAt:  now,
				ResourceVersion: 1,
				APIVersion:      "v1",
				Kind:            "network-sku",
				Ref:             "seca.network/v1/tenants/" + tenant + "/skus/hcloud-network",
				Tenant:          tenant,
				Region:          "global",
			},
			Spec: computeSKUSpec{VCPU: 0, RAM: 0},
		})
	}
}

func listImages(catalogProvider CatalogProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		tenant := r.PathValue("tenant")
		if tenant == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant is required", r.URL.Path)
			return
		}
		images, err := catalogProvider.ListCatalogImages(r.Context())
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		items := make([]imageResource, 0, len(images)+8)
		for _, rec := range runtimeResourceState.listImagesByTenant(tenant) {
			items = append(items, toRuntimeImageResource(rec, http.MethodGet, "active"))
		}
		for _, img := range images {
			if _, exists := runtimeResourceState.getImage(imageRef(tenant, img.Name)); exists {
				continue
			}
			items = append(items, imageResource{
				Metadata: resourceMetadata{
					Name:            img.Name,
					Provider:        "seca.storage/v1",
					Resource:        "tenants/" + tenant + "/images/" + img.Name,
					Verb:            http.MethodGet,
					CreatedAt:       now,
					LastModifiedAt:  now,
					ResourceVersion: 1,
					APIVersion:      "v1",
					Kind:            "image",
					Ref:             "seca.storage/v1/tenants/" + tenant + "/images/" + img.Name,
					Tenant:          tenant,
					Region:          "global",
				},
				Spec:   imageSpec{BlockStorageRef: refObject{Resource: "block-storages/" + img.Name}, CPUArchitecture: normalizeArchitecture(img.Architecture)},
				Status: imageStatus{State: "active"},
			})
		}
		respondJSON(w, http.StatusOK, imageIterator{Items: items, Metadata: responseMetaObject{Provider: "seca.storage/v1", Resource: "tenants/" + tenant + "/images", Verb: http.MethodGet}})
	}
}

func imageCRUD(catalogProvider CatalogProvider, conformanceMode bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getImage(catalogProvider)(w, r)
		case http.MethodPut:
			putImage(conformanceMode)(w, r)
		case http.MethodDelete:
			deleteImage(conformanceMode)(w, r)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func getImage(catalogProvider CatalogProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant := r.PathValue("tenant")
		name := strings.ToLower(r.PathValue("name"))
		if tenant == "" || name == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and image name are required", r.URL.Path)
			return
		}
		if rec, ok := runtimeResourceState.getImage(imageRef(tenant, name)); ok {
			respondJSON(w, http.StatusOK, toRuntimeImageResource(rec, http.MethodGet, "active"))
			return
		}
		img, err := catalogProvider.GetCatalogImage(r.Context(), name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if img == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "image not found", r.URL.Path)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		respondJSON(w, http.StatusOK, imageResource{
			Metadata: resourceMetadata{
				Name:            img.Name,
				Provider:        "seca.storage/v1",
				Resource:        "tenants/" + tenant + "/images/" + img.Name,
				Verb:            http.MethodGet,
				CreatedAt:       now,
				LastModifiedAt:  now,
				ResourceVersion: 1,
				APIVersion:      "v1",
				Kind:            "image",
				Ref:             "seca.storage/v1/tenants/" + tenant + "/images/" + img.Name,
				Tenant:          tenant,
				Region:          "global",
			},
			Spec:   imageSpec{BlockStorageRef: refObject{Resource: "block-storages/" + img.Name}, CPUArchitecture: normalizeArchitecture(img.Architecture)},
			Status: imageStatus{State: "active"},
		})
	}
}

func putImage(conformanceMode bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !conformanceMode {
			respondProblem(w, http.StatusNotImplemented, "http://secapi.cloud/errors/not-implemented", "Not Implemented", "image upload workflow is not implemented", r.URL.Path)
			return
		}
		tenant := r.PathValue("tenant")
		name := strings.ToLower(r.PathValue("name"))
		if tenant == "" || name == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and image name are required", r.URL.Path)
			return
		}
		var req imageResource
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		if strings.TrimSpace(req.Spec.BlockStorageRef.Resource) == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "spec.blockStorageRef is required", r.URL.Path)
			return
		}
		cpuArch := normalizeArchitecture(req.Spec.CPUArchitecture)
		region := strings.TrimSpace(req.Metadata.Region)
		if region == "" {
			region = "global"
		}

		now := time.Now().UTC().Format(time.RFC3339)
		rec, created := runtimeResourceState.upsertImage(imageRef(tenant, name), imageRuntimeRecord{
			Tenant:         tenant,
			Name:           name,
			Region:         region,
			Labels:         req.Labels,
			Spec:           imageSpec{BlockStorageRef: req.Spec.BlockStorageRef, CPUArchitecture: cpuArch},
			CreatedAt:      now,
			LastModifiedAt: now,
		})
		stateValue := "updating"
		code := http.StatusOK
		if created {
			stateValue = "creating"
			code = http.StatusCreated
		}
		respondJSON(w, code, toRuntimeImageResource(rec, http.MethodPut, stateValue))
	}
}

func deleteImage(conformanceMode bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !conformanceMode {
			respondProblem(w, http.StatusNotImplemented, "http://secapi.cloud/errors/not-implemented", "Not Implemented", "image upload workflow is not implemented", r.URL.Path)
			return
		}
		tenant := r.PathValue("tenant")
		name := strings.ToLower(r.PathValue("name"))
		if tenant == "" || name == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and image name are required", r.URL.Path)
			return
		}
		if _, ok := runtimeResourceState.getImage(imageRef(tenant, name)); !ok {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "image not found", r.URL.Path)
			return
		}
		runtimeResourceState.deleteImage(imageRef(tenant, name))
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func imageRef(tenant, name string) string {
	return strings.ToLower(strings.TrimSpace(tenant)) + "/" + strings.ToLower(strings.TrimSpace(name))
}

func toRuntimeImageResource(rec imageRuntimeRecord, verb, state string) imageResource {
	return imageResource{
		Metadata: resourceMetadata{
			Name:            rec.Name,
			Provider:        "seca.storage/v1",
			Resource:        "tenants/" + rec.Tenant + "/images/" + rec.Name,
			Verb:            verb,
			CreatedAt:       rec.CreatedAt,
			LastModifiedAt:  rec.LastModifiedAt,
			ResourceVersion: rec.ResourceVersion,
			APIVersion:      "v1",
			Kind:            "image",
			Ref:             "seca.storage/v1/tenants/" + rec.Tenant + "/images/" + rec.Name,
			Tenant:          rec.Tenant,
			Region:          rec.Region,
		},
		Labels: rec.Labels,
		Spec:   rec.Spec,
		Status: imageStatus{State: state},
	}
}

func toRegionResource(region hetzner.Region, now, verb string) regionResource {
	providers := make([]regionSpecVendor, 0, len(region.Providers))
	for _, provider := range region.Providers {
		providers = append(providers, regionSpecVendor{Name: provider.Name, Version: provider.Version, URL: provider.URL})
	}
	return regionResource{Metadata: resourceMetadata{Name: region.Name, Provider: "seca.region/v1", Resource: "regions/" + region.Name, Verb: verb, CreatedAt: now, LastModifiedAt: now, ResourceVersion: 1, APIVersion: "v1", Kind: "region", Ref: "seca.region/v1/regions/" + region.Name}, Spec: regionSpec{AvailableZones: region.Zones, Providers: providers}}
}

func normalizeArchitecture(arch string) string {
	switch strings.ToLower(arch) {
	case "x86", "x86_64", "amd64":
		return "amd64"
	case "arm", "arm64", "aarch64":
		return "arm64"
	default:
		return "amd64"
	}
}

func respondFromError(w http.ResponseWriter, err error, instance string) {
	if errors.Is(err, hetzner.ErrNotConfigured) {
		respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal-server-error", "Internal Server Error", "hetzner token is not configured", instance)
		return
	}
	var providerErr hetzner.ProviderError
	if errors.As(err, &providerErr) {
		switch providerErr.Code {
		case "invalid_request":
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", providerErr.Message, instance)
		case "not_found":
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", providerErr.Message, instance)
		default:
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal-server-error", "Internal Server Error", providerErr.Message, instance)
		}
		return
	}
	var apiErr hcloud.Error
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case hcloud.ErrorCodeUnauthorized, hcloud.ErrorCodeTokenReadonly:
			respondProblem(w, http.StatusUnauthorized, "http://secapi.cloud/errors/unauthorized", "Unauthorized", apiErr.Message, instance)
		case hcloud.ErrorCodeForbidden:
			respondProblem(w, http.StatusForbidden, "http://secapi.cloud/errors/forbidden", "Forbidden", apiErr.Message, instance)
		case hcloud.ErrorCodeNotFound:
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", apiErr.Message, instance)
		case hcloud.ErrorCodeConflict, hcloud.ErrorCodeLocked, hcloud.ErrorCodeResourceLocked, hcloud.ErrorCodeUniquenessError, hcloud.ErrorCodeVolumeAlreadyAttached:
			respondProblem(w, http.StatusConflict, "http://secapi.cloud/errors/resource-conflict", "Conflict", apiErr.Message, instance)
		case hcloud.ErrorCodeInvalidInput, hcloud.ErrorCodeJSONError, hcloud.ErrorCodeInvalidServerType, hcloud.ErrorCodeServerNotStopped:
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", apiErr.Message, instance)
		case hcloud.ErrorCodeRateLimitExceeded, hcloud.ErrorCodeResourceLimitExceeded:
			respondProblem(w, http.StatusTooManyRequests, "http://secapi.cloud/errors/rate-limited", "Too Many Requests", apiErr.Message, instance)
		case hcloud.ErrorCodeResourceUnavailable, hcloud.ErrorCodeMaintenance, hcloud.ErrorCodeRobotUnavailable, hcloud.ErrorCodeTimeout, hcloud.ErrorCodeNoSpaceLeftInLocation:
			respondProblem(w, http.StatusServiceUnavailable, "http://secapi.cloud/errors/provider-unavailable", "Service Unavailable", apiErr.Message, instance)
		case hcloud.ErrorUnsupportedError:
			respondProblem(w, http.StatusNotImplemented, "http://secapi.cloud/errors/not-implemented", "Not Implemented", apiErr.Message, instance)
		default:
			respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal-server-error", "Internal Server Error", apiErr.Message, instance)
		}
		return
	}
	respondProblem(w, http.StatusInternalServerError, "http://secapi.cloud/errors/internal-server-error", "Internal Server Error", err.Error(), instance)
}

func respondProblem(w http.ResponseWriter, code int, errType, title, detail, instance string) {
	respondJSON(w, code, problemResponse{Type: errType, Title: title, Status: code, Detail: detail, Instance: instance, Sources: []problemSource{}})
}

func respondJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
