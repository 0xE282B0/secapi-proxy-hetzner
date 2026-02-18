package httpserver

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/provider/hetzner"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

type blockStorageIterator struct {
	Items    []blockStorageResource `json:"items"`
	Metadata responseMetaObject     `json:"metadata"`
}

type blockStorageResource struct {
	Metadata resourceMetadata   `json:"metadata"`
	Spec     blockStorageSpec   `json:"spec"`
	Status   blockStorageStatus `json:"status"`
}

type blockStorageSpec struct {
	SizeGB int       `json:"sizeGB"`
	SkuRef refObject `json:"skuRef"`
}

type blockStorageStatus struct {
	State      string     `json:"state"`
	AttachedTo *refObject `json:"attachedTo,omitempty"`
	SizeGB     int        `json:"sizeGB"`
}

type blockStorageUpsertRequest struct {
	Spec struct {
		SizeGB         int        `json:"sizeGB"`
		SkuRef         *refObject `json:"skuRef,omitempty"`
		SourceImageRef *refObject `json:"sourceImageRef,omitempty"`
		AttachedTo     *refObject `json:"attachedTo,omitempty"`
	} `json:"spec"`
	Metadata struct {
		Region string `json:"region,omitempty"`
	} `json:"metadata,omitempty"`
}

type attachBlockStorageRequest struct {
	InstanceRef refObject `json:"instanceRef"`
}

func listBlockStorages(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		tenant, workspace, ok := scopeFromPath(w, r)
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		volumes, err := provider.ListBlockStorages(ctx)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		items := make([]blockStorageResource, 0, len(volumes))
		for _, volume := range volumes {
			spec, ok := runtimeResourceState.getBlockStorageSpec(blockStorageRef(tenant, workspace, volume.Name))
			if ok {
				items = append(items, toBlockStorageResource(tenant, workspace, volume, http.MethodGet, "active", &spec))
			} else {
				items = append(items, toBlockStorageResource(tenant, workspace, volume, http.MethodGet, "active", nil))
			}
			_ = store.UpsertResourceBinding(ctx, state.ResourceBinding{
				Tenant:      tenant,
				Workspace:   workspace,
				Kind:        "block-storage",
				SecaRef:     blockStorageRef(tenant, workspace, volume.Name),
				ProviderRef: "hetzner.cloud/volumes/" + volume.Name,
				Status:      "active",
			})
		}
		respondJSON(w, http.StatusOK, blockStorageIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.storage/v1", Resource: "tenants/" + tenant + "/workspaces/" + workspace + "/block-storages", Verb: http.MethodGet},
		})
	}
}

func blockStorageCRUD(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getBlockStorage(provider, store)(w, r)
		case http.MethodPut:
			putBlockStorage(provider, store)(w, r)
		case http.MethodDelete:
			deleteBlockStorage(provider, store)(w, r)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func getBlockStorage(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "block storage name is required")
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		volume, err := provider.GetBlockStorage(ctx, name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if volume == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "block storage not found", r.URL.Path)
			return
		}
		if err := store.UpsertResourceBinding(ctx, state.ResourceBinding{
			Tenant:      tenant,
			Workspace:   workspace,
			Kind:        "block-storage",
			SecaRef:     blockStorageRef(tenant, workspace, name),
			ProviderRef: "hetzner.cloud/volumes/" + name,
			Status:      "active",
		}); err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		spec, ok := runtimeResourceState.getBlockStorageSpec(blockStorageRef(tenant, workspace, name))
		if ok {
			respondJSON(w, http.StatusOK, toBlockStorageResource(tenant, workspace, *volume, http.MethodGet, "active", &spec))
			return
		}
		respondJSON(w, http.StatusOK, toBlockStorageResource(tenant, workspace, *volume, http.MethodGet, "active", nil))
	}
}

func putBlockStorage(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "block storage name is required")
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		var reqBody blockStorageUpsertRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		requestedSizeGB := reqBody.Spec.SizeGB
		if requestedSizeGB <= 0 {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "spec.sizeGB must be > 0", r.URL.Path)
			return
		}
		if reqBody.Spec.SkuRef == nil || reqBody.Spec.SkuRef.Resource == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "spec.skuRef.resource is required", r.URL.Path)
			return
		}
		providerSizeGB := normalizeProviderBlockStorageSizeGB(requestedSizeGB)
		attachTo := ""
		if reqBody.Spec.AttachedTo != nil {
			attachTo = resourceNameFromRef(reqBody.Spec.AttachedTo.Resource)
		}
		volume, created, actionID, err := provider.CreateOrUpdateBlockStorage(ctx, hetzner.BlockStorageCreateRequest{
			Name:     name,
			SizeGB:   providerSizeGB,
			Region:   reqBody.Metadata.Region,
			AttachTo: attachTo,
		})
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if err := store.UpsertResourceBinding(ctx, state.ResourceBinding{
			Tenant:      tenant,
			Workspace:   workspace,
			Kind:        "block-storage",
			SecaRef:     blockStorageRef(tenant, workspace, name),
			ProviderRef: "hetzner.cloud/volumes/" + name,
			Status:      "active",
		}); err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if actionID != "" {
			if err := store.CreateOperation(ctx, state.OperationRecord{
				OperationID:      operationID("block-storage-upsert", name),
				SecaRef:          blockStorageRef(tenant, workspace, name),
				ProviderActionID: actionID,
				Phase:            "accepted",
			}); err != nil {
				respondFromError(w, err, r.URL.Path)
				return
			}
		}
		code := http.StatusOK
		stateValue := "updating"
		if created {
			code = http.StatusCreated
			stateValue = "creating"
		}
		spec := blockStorageSpec{
			SizeGB: requestedSizeGB,
			SkuRef: *reqBody.Spec.SkuRef,
		}
		runtimeResourceState.setBlockStorageSpec(blockStorageRef(tenant, workspace, name), spec)
		respondJSON(w, code, toBlockStorageResource(tenant, workspace, *volume, http.MethodPut, stateValue, &spec))
	}
}

func deleteBlockStorage(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "block storage name is required")
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		deleted, err := provider.DeleteBlockStorage(ctx, name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if !deleted {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "block storage not found", r.URL.Path)
			return
		}
		_ = store.DeleteResourceBinding(ctx, blockStorageRef(tenant, workspace, name))
		runtimeResourceState.deleteBlockStorageSpec(blockStorageRef(tenant, workspace, name))
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func attachBlockStorage(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only POST is supported", r.URL.Path)
			return
		}
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "block storage name is required")
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		var reqBody attachBlockStorageRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		instanceName := resourceNameFromRef(reqBody.InstanceRef.Resource)
		if instanceName == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "instanceRef.resource is required", r.URL.Path)
			return
		}
		found, actionID, err := provider.AttachBlockStorage(ctx, name, instanceName)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if !found {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "block storage not found", r.URL.Path)
			return
		}
		if err := store.CreateOperation(ctx, state.OperationRecord{
			OperationID:      operationID("block-storage-attach", name),
			SecaRef:          blockStorageRef(tenant, workspace, name),
			ProviderActionID: actionID,
			Phase:            "accepted",
		}); err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func detachBlockStorage(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only POST is supported", r.URL.Path)
			return
		}
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "block storage name is required")
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		found, actionID, err := provider.DetachBlockStorage(ctx, name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if !found {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "block storage not found", r.URL.Path)
			return
		}
		if err := store.CreateOperation(ctx, state.OperationRecord{
			OperationID:      operationID("block-storage-detach", name),
			SecaRef:          blockStorageRef(tenant, workspace, name),
			ProviderActionID: actionID,
			Phase:            "accepted",
		}); err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func toBlockStorageResource(tenant, workspace string, volume hetzner.BlockStorage, verb, state string, specOverride *blockStorageSpec) blockStorageResource {
	now := time.Now().UTC().Format(time.RFC3339)
	var attachedTo *refObject
	if volume.AttachedTo != "" {
		attachedTo = &refObject{Resource: "instances/" + volume.AttachedTo}
	}
	spec := blockStorageSpec{
		SizeGB: volume.SizeGB,
		SkuRef: refObject{Resource: "skus/hcloud-volume"},
	}
	if specOverride != nil {
		spec = *specOverride
	}
	return blockStorageResource{
		Metadata: resourceMetadata{
			Name:            volume.Name,
			Provider:        "seca.storage/v1",
			Resource:        "tenants/" + tenant + "/workspaces/" + workspace + "/block-storages/" + volume.Name,
			Verb:            verb,
			CreatedAt:       now,
			LastModifiedAt:  now,
			ResourceVersion: 1,
			APIVersion:      "v1",
			Kind:            "block-storage",
			Ref:             blockStorageRef(tenant, workspace, volume.Name),
			Tenant:          tenant,
			Workspace:       workspace,
			Region:          defaultRegion(volume.Region),
		},
		Spec: spec,
		Status: blockStorageStatus{
			State:      state,
			AttachedTo: attachedTo,
			SizeGB:     volume.SizeGB,
		},
	}
}
