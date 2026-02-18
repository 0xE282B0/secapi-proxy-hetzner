package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/provider/hetzner"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/state"
)

type instanceIterator struct {
	Items    []instanceResource `json:"items"`
	Metadata responseMetaObject `json:"metadata"`
}

type instanceResource struct {
	Metadata resourceMetadata `json:"metadata"`
	Spec     instanceSpec     `json:"spec"`
	Status   instanceStatus   `json:"status"`
}

type instanceSpec struct {
	SkuRef   refObject `json:"skuRef"`
	ImageRef refObject `json:"imageRef"`
	Zone     string    `json:"zone,omitempty"`
}

type instanceStatus struct {
	State      string `json:"state"`
	PowerState string `json:"powerState"`
}

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

type instanceUpsertRequest struct {
	Spec struct {
		SkuRef         refObject  `json:"skuRef"`
		ImageRef       *refObject `json:"imageRef,omitempty"`
		SourceImageRef *refObject `json:"sourceImageRef,omitempty"`
		Zone           string     `json:"zone,omitempty"`
		UserData       string     `json:"userData,omitempty"`
	} `json:"spec"`
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

func listInstances(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET is supported", r.URL.Path)
			return
		}
		tenant, workspace, ok := scopeFromPath(w, r)
		if !ok {
			return
		}

		instances, err := provider.ListInstances(r.Context())
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}

		items := make([]instanceResource, 0, len(instances))
		for _, instance := range instances {
			items = append(items, toInstanceResource(tenant, workspace, instance, http.MethodGet))
			_ = store.UpsertResourceBinding(r.Context(), state.ResourceBinding{
				Tenant:      tenant,
				Workspace:   workspace,
				Kind:        "instance",
				SecaRef:     computeInstanceRef(tenant, workspace, instance.Name),
				ProviderRef: "hetzner.cloud/servers/" + instance.Name,
				Status:      "active",
			})
		}

		respondJSON(w, http.StatusOK, instanceIterator{
			Items:    items,
			Metadata: responseMetaObject{Provider: "seca.compute/v1", Resource: "tenants/" + tenant + "/workspaces/" + workspace + "/instances", Verb: "list"},
		})
	}
}

func instanceCRUD(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getInstance(provider, store)(w, r)
		case http.MethodPut:
			putInstance(provider, store)(w, r)
		case http.MethodDelete:
			deleteInstance(provider, store)(w, r)
		default:
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only GET, PUT and DELETE are supported", r.URL.Path)
		}
	}
}

func getInstance(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "instance name is required")
		if !ok {
			return
		}
		instance, err := provider.GetInstance(r.Context(), name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if instance == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "instance not found", r.URL.Path)
			return
		}
		if err := store.UpsertResourceBinding(r.Context(), state.ResourceBinding{
			Tenant:      tenant,
			Workspace:   workspace,
			Kind:        "instance",
			SecaRef:     computeInstanceRef(tenant, workspace, name),
			ProviderRef: "hetzner.cloud/servers/" + name,
			Status:      "active",
		}); err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		respondJSON(w, http.StatusOK, toInstanceResource(tenant, workspace, *instance, http.MethodGet))
	}
}

func putInstance(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "instance name is required")
		if !ok {
			return
		}
		var reqBody instanceUpsertRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		skuName := resourceNameFromRef(reqBody.Spec.SkuRef.Resource)
		if skuName == "" {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "spec.skuRef.resource is required", r.URL.Path)
			return
		}
		imageName := ""
		if reqBody.Spec.ImageRef != nil {
			imageName = resourceNameFromRef(reqBody.Spec.ImageRef.Resource)
		}
		if imageName == "" && reqBody.Spec.SourceImageRef != nil {
			imageName = resourceNameFromRef(reqBody.Spec.SourceImageRef.Resource)
		}
		if imageName == "" {
			imageName = "ubuntu-24.04"
		}

		instance, created, actionID, err := provider.CreateOrUpdateInstance(r.Context(), hetzner.InstanceCreateRequest{
			Name:      name,
			SKUName:   skuName,
			ImageName: imageName,
			Region:    regionFromZone(reqBody.Spec.Zone),
			UserData:  reqBody.Spec.UserData,
		})
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if err := store.UpsertResourceBinding(r.Context(), state.ResourceBinding{
			Tenant:      tenant,
			Workspace:   workspace,
			Kind:        "instance",
			SecaRef:     computeInstanceRef(tenant, workspace, name),
			ProviderRef: "hetzner.cloud/servers/" + name,
			Status:      "active",
		}); err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if actionID != "" {
			if err := store.CreateOperation(r.Context(), state.OperationRecord{
				OperationID:      operationID("instance-upsert", name),
				SecaRef:          computeInstanceRef(tenant, workspace, name),
				ProviderActionID: actionID,
				Phase:            "accepted",
			}); err != nil {
				respondFromError(w, err, r.URL.Path)
				return
			}
		}
		code := http.StatusOK
		verb := http.MethodPut
		if created {
			code = http.StatusCreated
			verb = http.MethodPut
		}
		respondJSON(w, code, toInstanceResource(tenant, workspace, *instance, verb))
	}
}

func deleteInstance(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "instance name is required")
		if !ok {
			return
		}
		deleted, actionID, err := provider.DeleteInstance(r.Context(), name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if !deleted {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "instance not found", r.URL.Path)
			return
		}
		_ = store.DeleteResourceBinding(r.Context(), computeInstanceRef(tenant, workspace, name))
		if actionID != "" {
			_ = store.CreateOperation(r.Context(), state.OperationRecord{
				OperationID:      operationID("instance-delete", name),
				SecaRef:          computeInstanceRef(tenant, workspace, name),
				ProviderActionID: actionID,
				Phase:            "accepted",
			})
		}
		respondJSON(w, http.StatusNoContent, map[string]any{})
	}
}

func startInstance(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return instanceAction(provider.StartInstance, "instance-start", provider, store)
}

func stopInstance(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return instanceAction(provider.StopInstance, "instance-stop", provider, store)
}

func restartInstance(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return instanceAction(provider.RestartInstance, "instance-restart", provider, store)
}

func instanceAction(action func(ctx context.Context, name string) (bool, string, error), phase string, _ ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only POST is supported", r.URL.Path)
			return
		}
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "instance name is required")
		if !ok {
			return
		}
		found, actionID, err := action(r.Context(), name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if !found {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "instance not found", r.URL.Path)
			return
		}
		if err := store.CreateOperation(r.Context(), state.OperationRecord{
			OperationID:      operationID(phase, name),
			SecaRef:          computeInstanceRef(tenant, workspace, name),
			ProviderActionID: actionID,
			Phase:            "accepted",
		}); err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
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
		volumes, err := provider.ListBlockStorages(r.Context())
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		items := make([]blockStorageResource, 0, len(volumes))
		for _, volume := range volumes {
			items = append(items, toBlockStorageResource(tenant, workspace, volume, http.MethodGet))
			_ = store.UpsertResourceBinding(r.Context(), state.ResourceBinding{
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
			Metadata: responseMetaObject{Provider: "seca.storage/v1", Resource: "tenants/" + tenant + "/workspaces/" + workspace + "/block-storages", Verb: "list"},
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
		volume, err := provider.GetBlockStorage(r.Context(), name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if volume == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "block storage not found", r.URL.Path)
			return
		}
		if err := store.UpsertResourceBinding(r.Context(), state.ResourceBinding{
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
		respondJSON(w, http.StatusOK, toBlockStorageResource(tenant, workspace, *volume, http.MethodGet))
	}
}

func putBlockStorage(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "block storage name is required")
		if !ok {
			return
		}
		var reqBody blockStorageUpsertRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "invalid json body", r.URL.Path)
			return
		}
		if reqBody.Spec.SizeGB <= 0 {
			respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "spec.sizeGB must be > 0", r.URL.Path)
			return
		}
		attachTo := ""
		if reqBody.Spec.AttachedTo != nil {
			attachTo = resourceNameFromRef(reqBody.Spec.AttachedTo.Resource)
		}
		volume, created, actionID, err := provider.CreateOrUpdateBlockStorage(r.Context(), hetzner.BlockStorageCreateRequest{
			Name:     name,
			SizeGB:   reqBody.Spec.SizeGB,
			Region:   reqBody.Metadata.Region,
			AttachTo: attachTo,
		})
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if err := store.UpsertResourceBinding(r.Context(), state.ResourceBinding{
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
			if err := store.CreateOperation(r.Context(), state.OperationRecord{
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
		verb := http.MethodPut
		if created {
			code = http.StatusCreated
			verb = http.MethodPut
		}
		respondJSON(w, code, toBlockStorageResource(tenant, workspace, *volume, verb))
	}
}

func deleteBlockStorage(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "block storage name is required")
		if !ok {
			return
		}
		deleted, err := provider.DeleteBlockStorage(r.Context(), name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if !deleted {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "block storage not found", r.URL.Path)
			return
		}
		_ = store.DeleteResourceBinding(r.Context(), blockStorageRef(tenant, workspace, name))
		respondJSON(w, http.StatusNoContent, map[string]any{})
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
		found, actionID, err := provider.AttachBlockStorage(r.Context(), name, instanceName)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if !found {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "block storage not found", r.URL.Path)
			return
		}
		if err := store.CreateOperation(r.Context(), state.OperationRecord{
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
		found, actionID, err := provider.DetachBlockStorage(r.Context(), name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if !found {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "block storage not found", r.URL.Path)
			return
		}
		if err := store.CreateOperation(r.Context(), state.OperationRecord{
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

func toInstanceResource(tenant, workspace string, instance hetzner.Instance, verb string) instanceResource {
	now := time.Now().UTC().Format(time.RFC3339)
	return instanceResource{
		Metadata: resourceMetadata{
			Name:            instance.Name,
			Provider:        "seca.compute/v1",
			Resource:        "tenants/" + tenant + "/workspaces/" + workspace + "/instances/" + instance.Name,
			Verb:            verb,
			CreatedAt:       now,
			LastModifiedAt:  now,
			ResourceVersion: 1,
			APIVersion:      "v1",
			Kind:            "instance",
			Ref:             computeInstanceRef(tenant, workspace, instance.Name),
			Tenant:          tenant,
			Workspace:       workspace,
			Region:          defaultRegion(instance.Region),
		},
		Spec: instanceSpec{
			SkuRef:   refObject{Resource: "skus/" + instance.SKUName},
			ImageRef: refObject{Resource: "images/" + instance.ImageName},
			Zone:     instance.Region,
		},
		Status: instanceStatus{
			State:      "active",
			PowerState: instance.PowerState,
		},
	}
}

func toBlockStorageResource(tenant, workspace string, volume hetzner.BlockStorage, verb string) blockStorageResource {
	now := time.Now().UTC().Format(time.RFC3339)
	var attachedTo *refObject
	if volume.AttachedTo != "" {
		attachedTo = &refObject{Resource: "instances/" + volume.AttachedTo}
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
		Spec: blockStorageSpec{
			SizeGB: volume.SizeGB,
			SkuRef: refObject{Resource: "skus/default"},
		},
		Status: blockStorageStatus{
			State:      "active",
			AttachedTo: attachedTo,
			SizeGB:     volume.SizeGB,
		},
	}
}

func scopeFromPath(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	tenant := r.PathValue("tenant")
	workspace := r.PathValue("workspace")
	if tenant == "" || workspace == "" {
		respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", "tenant and workspace are required", r.URL.Path)
		return "", "", false
	}
	return tenant, workspace, true
}

func scopedNameFromPath(w http.ResponseWriter, r *http.Request, nameErr string) (string, string, string, bool) {
	tenant, workspace, ok := scopeFromPath(w, r)
	if !ok {
		return "", "", "", false
	}
	name := strings.ToLower(r.PathValue("name"))
	if name == "" {
		respondProblem(w, http.StatusBadRequest, "http://secapi.cloud/errors/invalid-request", "Bad Request", nameErr, r.URL.Path)
		return "", "", "", false
	}
	return tenant, workspace, name, true
}

func resourceNameFromRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	parts := strings.Split(ref, "/")
	return strings.ToLower(parts[len(parts)-1])
}

func regionFromZone(zone string) string {
	if zone == "" {
		return ""
	}
	if idx := strings.Index(zone, "-dc"); idx > 0 {
		return strings.ToLower(zone[:idx])
	}
	return strings.ToLower(zone)
}

func defaultRegion(value string) string {
	if value == "" {
		return "global"
	}
	return strings.ToLower(value)
}

func computeInstanceRef(tenant, workspace, name string) string {
	return "seca.compute/v1/tenants/" + tenant + "/workspaces/" + workspace + "/instances/" + name
}

func blockStorageRef(tenant, workspace, name string) string {
	return "seca.storage/v1/tenants/" + tenant + "/workspaces/" + workspace + "/block-storages/" + name
}

func operationID(prefix, name string) string {
	return fmt.Sprintf("%s-%s-%d", prefix, name, time.Now().UnixNano())
}
