package httpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/config"
	"github.com/eu-sovereign-cloud/secapi-proxy-hetzner/internal/provider/hetzner"
)

type fakeComputeProvider struct {
	createReq    *hetzner.InstanceCreateRequest
	getInstance  *hetzner.Instance
	deleteName   string
	syncName     string
	syncNetworks []string
}

func (f *fakeComputeProvider) ListInstances(context.Context) ([]hetzner.Instance, error) {
	return nil, nil
}

func (f *fakeComputeProvider) GetInstance(context.Context, string) (*hetzner.Instance, error) {
	return f.getInstance, nil
}

func (f *fakeComputeProvider) CreateOrUpdateInstance(_ context.Context, req hetzner.InstanceCreateRequest) (*hetzner.Instance, bool, string, error) {
	r := req
	f.createReq = &r
	return &hetzner.Instance{Name: req.Name}, true, "", nil
}

func (f *fakeComputeProvider) DeleteInstance(_ context.Context, name string) (bool, string, error) {
	f.deleteName = name
	return true, "", nil
}

func (f *fakeComputeProvider) StartInstance(context.Context, string) (bool, string, error) {
	return true, "", nil
}

func (f *fakeComputeProvider) StopInstance(context.Context, string) (bool, string, error) {
	return true, "", nil
}

func (f *fakeComputeProvider) RestartInstance(context.Context, string) (bool, string, error) {
	return true, "", nil
}

func (f *fakeComputeProvider) AttachInstanceToNetwork(context.Context, string, string) (bool, string, error) {
	return true, "", nil
}

func (f *fakeComputeProvider) SyncInstanceNetworks(_ context.Context, instanceName string, networkNames []string) error {
	f.syncName = instanceName
	f.syncNetworks = append([]string(nil), networkNames...)
	return nil
}

func (f *fakeComputeProvider) ListBlockStorages(context.Context) ([]hetzner.BlockStorage, error) {
	return nil, nil
}

func (f *fakeComputeProvider) GetBlockStorage(context.Context, string) (*hetzner.BlockStorage, error) {
	return nil, nil
}

func (f *fakeComputeProvider) CreateOrUpdateBlockStorage(context.Context, hetzner.BlockStorageCreateRequest) (*hetzner.BlockStorage, bool, string, error) {
	return nil, false, "", nil
}

func (f *fakeComputeProvider) DeleteBlockStorage(context.Context, string) (bool, error) {
	return true, nil
}

func (f *fakeComputeProvider) AttachBlockStorage(context.Context, string, string) (bool, string, error) {
	return true, "", nil
}

func (f *fakeComputeProvider) DetachBlockStorage(context.Context, string) (bool, string, error) {
	return true, "", nil
}

func TestReconcileInternetGatewayProviderCreateAndSync(t *testing.T) {
	t.Parallel()

	fake := &fakeComputeProvider{
		getInstance: &hetzner.Instance{Name: "seca-igw-ws1-igw1"},
	}
	cfg := config.Config{InternetGatewayNATVM: true}
	payload := internetGatewayBindingPayload{
		Name:        "igw1",
		Region:      "nbg1",
		Labels:      map[string]string{"env": "dev"},
		Spec:        internetGatewaySpec{},
		Networks:    []string{"network-a", "network-b"},
		RouteTables: []string{"rt-a"},
	}

	ref, err := reconcileInternetGatewayProvider(context.Background(), nil, fake, cfg, "dev", "ws1", payload)
	if err != nil {
		t.Fatalf("reconcileInternetGatewayProvider returned error: %v", err)
	}
	if ref != "instances/seca-igw-ws1-igw1" {
		t.Fatalf("unexpected provider ref: %s", ref)
	}
	if fake.createReq == nil {
		t.Fatal("expected instance create request")
	}
	if fake.createReq.Name != "seca-igw-ws1-igw1" {
		t.Fatalf("unexpected instance name: %s", fake.createReq.Name)
	}
	if fake.createReq.Region != "nbg1" {
		t.Fatalf("unexpected region: %s", fake.createReq.Region)
	}
	if !strings.Contains(fake.createReq.UserData, "#cloud-config") {
		t.Fatal("expected cloud-init user data")
	}
	if !strings.Contains(fake.createReq.UserData, "ip_forward=1") {
		t.Fatal("expected ipv4 forwarding setup in user data")
	}
	if fake.syncName != "seca-igw-ws1-igw1" {
		t.Fatalf("unexpected sync instance: %s", fake.syncName)
	}
	if len(fake.syncNetworks) != 2 {
		t.Fatalf("unexpected sync networks length: %d", len(fake.syncNetworks))
	}
	if fake.deleteName != "" {
		t.Fatalf("did not expect delete call, got: %s", fake.deleteName)
	}
}

func TestReconcileInternetGatewayProviderCleanup(t *testing.T) {
	t.Parallel()

	fake := &fakeComputeProvider{}
	cfg := config.Config{InternetGatewayNATVM: true}
	payload := internetGatewayBindingPayload{
		Name:        "igw2",
		Region:      "nbg1",
		RouteTables: nil,
	}

	ref, err := reconcileInternetGatewayProvider(context.Background(), nil, fake, cfg, "dev", "ws1", payload)
	if err != nil {
		t.Fatalf("reconcileInternetGatewayProvider returned error: %v", err)
	}
	if ref != "" {
		t.Fatalf("expected empty provider ref, got: %s", ref)
	}
	if fake.deleteName != "seca-igw-ws1-igw2" {
		t.Fatalf("unexpected deleted instance: %s", fake.deleteName)
	}
	if fake.createReq != nil {
		t.Fatal("did not expect create request")
	}
}
