package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
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
	SkuRef     refObject       `json:"skuRef"`
	ImageRef   refObject       `json:"imageRef"`
	BootVolume volumeReference `json:"bootVolume,omitempty"`
	Zone       string          `json:"zone,omitempty"`
}

type volumeReference struct {
	DeviceRef refObject `json:"deviceRef"`
}

type instanceStatus struct {
	State      string `json:"state"`
	PowerState string `json:"powerState"`
}

type instanceUpsertRequest struct {
	Labels map[string]string `json:"labels,omitempty"`
	Spec struct {
		SkuRef         refObject  `json:"skuRef"`
		ImageRef       *refObject `json:"imageRef,omitempty"`
		SourceImageRef *refObject `json:"sourceImageRef,omitempty"`
		BootVolume     *struct {
			DeviceRef refObject `json:"deviceRef"`
		} `json:"bootVolume,omitempty"`
		Zone     string `json:"zone,omitempty"`
		UserData string `json:"userData,omitempty"`
	} `json:"spec"`
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
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}

		instances, err := provider.ListInstances(ctx)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}

		items := make([]instanceResource, 0, len(instances))
		for _, instance := range instances {
			spec, ok := runtimeResourceState.getInstanceSpec(computeInstanceRef(tenant, workspace, instance.Name))
			if ok {
				items = append(items, toInstanceResource(tenant, workspace, instance, http.MethodGet, "active", &spec))
			} else {
				items = append(items, toInstanceResource(tenant, workspace, instance, http.MethodGet, "active", nil))
			}
			_ = store.UpsertResourceBinding(ctx, state.ResourceBinding{
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
			Metadata: responseMetaObject{Provider: "seca.compute/v1", Resource: "tenants/" + tenant + "/workspaces/" + workspace + "/instances", Verb: http.MethodGet},
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
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		instance, err := provider.GetInstance(ctx, name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if instance == nil {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "instance not found", r.URL.Path)
			return
		}
		if err := store.UpsertResourceBinding(ctx, state.ResourceBinding{
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
		spec, ok := runtimeResourceState.getInstanceSpec(computeInstanceRef(tenant, workspace, name))
		if ok {
			respondJSON(w, http.StatusOK, toInstanceResource(tenant, workspace, *instance, http.MethodGet, "active", &spec))
			return
		}
		respondJSON(w, http.StatusOK, toInstanceResource(tenant, workspace, *instance, http.MethodGet, "active", nil))
	}
}

func putInstance(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "instance name is required")
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
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

		instance, created, actionID, err := provider.CreateOrUpdateInstance(ctx, hetzner.InstanceCreateRequest{
			Name:      name,
			SKUName:   skuName,
			ImageName: imageName,
			Region:    regionFromZone(reqBody.Spec.Zone),
			UserData:  reqBody.Spec.UserData,
			Labels: withSecaProviderLabels(
				reqBody.Labels,
				tenant,
				workspace,
				"instance",
				name,
				computeInstanceRef(tenant, workspace, name),
			),
		})
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if err := store.UpsertResourceBinding(ctx, state.ResourceBinding{
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
			if err := store.CreateOperation(ctx, state.OperationRecord{
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
		stateValue := "updating"
		if created {
			code = http.StatusCreated
			stateValue = "creating"
		}
		storedSpec := instanceSpec{
			SkuRef:     reqBody.Spec.SkuRef,
			ImageRef:   refObject{Resource: "images/" + imageName},
			BootVolume: volumeReference{},
			Zone:       reqBody.Spec.Zone,
		}
		if reqBody.Spec.BootVolume != nil {
			storedSpec.BootVolume.DeviceRef = reqBody.Spec.BootVolume.DeviceRef
		}
		runtimeResourceState.setInstanceSpec(computeInstanceRef(tenant, workspace, name), storedSpec)
		respondJSON(w, code, toInstanceResource(tenant, workspace, *instance, http.MethodPut, stateValue, &storedSpec))
	}
}

func deleteInstance(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "instance name is required")
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		deleted, actionID, err := provider.DeleteInstance(ctx, name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if !deleted {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "instance not found", r.URL.Path)
			return
		}
		_ = store.DeleteResourceBinding(ctx, computeInstanceRef(tenant, workspace, name))
		runtimeResourceState.deleteInstanceSpec(computeInstanceRef(tenant, workspace, name))
		if actionID != "" {
			_ = store.CreateOperation(ctx, state.OperationRecord{
				OperationID:      operationID("instance-delete", name),
				SecaRef:          computeInstanceRef(tenant, workspace, name),
				ProviderActionID: actionID,
				Phase:            "accepted",
			})
		}
		respondJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	}
}

func startInstance(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return instanceAction(provider.StartInstance, "instance-start", store)
}

func stopInstance(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return instanceAction(provider.StopInstance, "instance-stop", store)
}

func restartInstance(provider ComputeStorageProvider, store *state.Store) http.HandlerFunc {
	return instanceAction(provider.RestartInstance, "instance-restart", store)
}

func instanceAction(action func(ctx context.Context, name string) (bool, string, error), phase string, store *state.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			respondProblem(w, http.StatusMethodNotAllowed, "http://secapi.cloud/errors/invalid-request", "Method Not Allowed", "Only POST is supported", r.URL.Path)
			return
		}
		tenant, workspace, name, ok := scopedNameFromPath(w, r, "instance name is required")
		if !ok {
			return
		}
		ctx, ok := workspaceExecutionContext(w, r, store, tenant, workspace)
		if !ok {
			return
		}
		found, actionID, err := action(ctx, name)
		if err != nil {
			respondFromError(w, err, r.URL.Path)
			return
		}
		if !found {
			respondProblem(w, http.StatusNotFound, "http://secapi.cloud/errors/resource-not-found", "Not Found", "instance not found", r.URL.Path)
			return
		}
		if err := store.CreateOperation(ctx, state.OperationRecord{
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

func toInstanceResource(tenant, workspace string, instance hetzner.Instance, verb, state string, specOverride *instanceSpec) instanceResource {
	now := time.Now().UTC().Format(time.RFC3339)
	spec := instanceSpec{
		SkuRef:     refObject{Resource: "skus/" + instance.SKUName},
		ImageRef:   refObject{Resource: "images/" + instance.ImageName},
		BootVolume: volumeReference{},
		Zone:       instance.Region,
	}
	if specOverride != nil {
		spec = *specOverride
	}
	region := defaultRegion(instance.Region)
	if spec.Zone != "" {
		region = defaultRegion(regionFromZone(spec.Zone))
	}
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
			Region:          region,
		},
		Spec: spec,
		Status: instanceStatus{
			State:      state,
			PowerState: instance.PowerState,
		},
	}
}
