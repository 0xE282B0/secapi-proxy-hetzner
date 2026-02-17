package hetzner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

type Instance struct {
	Name       string
	SKUName    string
	ImageName  string
	Region     string
	PowerState string
	CreatedAt  time.Time
}

type InstanceCreateRequest struct {
	Name      string
	SKUName   string
	ImageName string
	Region    string
	UserData  string
}

type BlockStorage struct {
	Name       string
	SizeGB     int
	Region     string
	AttachedTo string
	CreatedAt  time.Time
}

type BlockStorageCreateRequest struct {
	Name     string
	SizeGB   int
	Region   string
	AttachTo string
}

func (s *RegionService) ListInstances(ctx context.Context) ([]Instance, error) {
	if !s.configured {
		return nil, ErrNotConfigured
	}
	servers, err := s.client.Server.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Instance, 0, len(servers))
	for _, server := range servers {
		if server == nil {
			continue
		}
		out = append(out, instanceFromServer(server))
	}
	return out, nil
}

func (s *RegionService) GetInstance(ctx context.Context, name string) (*Instance, error) {
	if !s.configured {
		return nil, ErrNotConfigured
	}
	server, _, err := s.client.Server.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, nil
	}
	instance := instanceFromServer(server)
	return &instance, nil
}

func (s *RegionService) CreateOrUpdateInstance(ctx context.Context, req InstanceCreateRequest) (*Instance, bool, string, error) {
	if !s.configured {
		return nil, false, "", ErrNotConfigured
	}

	current, _, err := s.client.Server.GetByName(ctx, req.Name)
	if err != nil {
		return nil, false, "", err
	}
	if current != nil {
		instance := instanceFromServer(current)
		return &instance, false, "", nil
	}

	serverType, _, err := s.client.ServerType.GetByName(ctx, req.SKUName)
	if err != nil {
		return nil, false, "", err
	}
	if serverType == nil {
		return nil, false, "", notFoundError(fmt.Sprintf("compute sku %q not found", req.SKUName))
	}

	image, _, err := s.client.Image.GetByName(ctx, req.ImageName)
	if err != nil {
		return nil, false, "", err
	}
	if image == nil {
		return nil, false, "", notFoundError(fmt.Sprintf("image %q not found", req.ImageName))
	}

	createOpts := hcloud.ServerCreateOpts{
		Name:       req.Name,
		ServerType: serverType,
		Image:      image,
		UserData:   req.UserData,
	}
	if req.Region != "" {
		location, _, locErr := s.client.Location.GetByName(ctx, req.Region)
		if locErr != nil {
			return nil, false, "", locErr
		}
		if location == nil {
			return nil, false, "", notFoundError(fmt.Sprintf("region %q not found", req.Region))
		}
		createOpts.Location = location
	}

	result, _, err := s.client.Server.Create(ctx, createOpts)
	if err != nil {
		return nil, false, "", err
	}
	if result.Server == nil {
		return nil, false, "", fmt.Errorf("hetzner returned empty server")
	}

	actionID := ""
	if result.Action != nil {
		actionID = fmt.Sprintf("%d", result.Action.ID)
	}
	instance := instanceFromServer(result.Server)
	return &instance, true, actionID, nil
}

func (s *RegionService) DeleteInstance(ctx context.Context, name string) (bool, string, error) {
	if !s.configured {
		return false, "", ErrNotConfigured
	}
	server, _, err := s.client.Server.GetByName(ctx, name)
	if err != nil {
		return false, "", err
	}
	if server == nil {
		return false, "", nil
	}
	result, _, err := s.client.Server.DeleteWithResult(ctx, server)
	if err != nil {
		return false, "", err
	}
	actionID := ""
	if result != nil && result.Action != nil {
		actionID = fmt.Sprintf("%d", result.Action.ID)
	}
	return true, actionID, nil
}

func (s *RegionService) StartInstance(ctx context.Context, name string) (bool, string, error) {
	server, err := s.getServerByName(ctx, name)
	if err != nil {
		return false, "", err
	}
	if server == nil {
		return false, "", nil
	}
	action, _, err := s.client.Server.Poweron(ctx, server)
	if err != nil {
		return false, "", err
	}
	return true, fmt.Sprintf("%d", action.ID), nil
}

func (s *RegionService) StopInstance(ctx context.Context, name string) (bool, string, error) {
	server, err := s.getServerByName(ctx, name)
	if err != nil {
		return false, "", err
	}
	if server == nil {
		return false, "", nil
	}
	action, _, err := s.client.Server.Poweroff(ctx, server)
	if err != nil {
		return false, "", err
	}
	return true, fmt.Sprintf("%d", action.ID), nil
}

func (s *RegionService) RestartInstance(ctx context.Context, name string) (bool, string, error) {
	server, err := s.getServerByName(ctx, name)
	if err != nil {
		return false, "", err
	}
	if server == nil {
		return false, "", nil
	}
	action, _, err := s.client.Server.Reboot(ctx, server)
	if err != nil {
		return false, "", err
	}
	return true, fmt.Sprintf("%d", action.ID), nil
}

func (s *RegionService) ListBlockStorages(ctx context.Context) ([]BlockStorage, error) {
	if !s.configured {
		return nil, ErrNotConfigured
	}
	volumes, err := s.client.Volume.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]BlockStorage, 0, len(volumes))
	for _, volume := range volumes {
		if volume == nil {
			continue
		}
		out = append(out, blockStorageFromVolume(volume))
	}
	return out, nil
}

func (s *RegionService) GetBlockStorage(ctx context.Context, name string) (*BlockStorage, error) {
	if !s.configured {
		return nil, ErrNotConfigured
	}
	volume, _, err := s.client.Volume.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if volume == nil {
		return nil, nil
	}
	block := blockStorageFromVolume(volume)
	return &block, nil
}

func (s *RegionService) CreateOrUpdateBlockStorage(ctx context.Context, req BlockStorageCreateRequest) (*BlockStorage, bool, string, error) {
	if !s.configured {
		return nil, false, "", ErrNotConfigured
	}
	current, _, err := s.client.Volume.GetByName(ctx, req.Name)
	if err != nil {
		return nil, false, "", err
	}
	if current != nil {
		block := blockStorageFromVolume(current)
		return &block, false, "", nil
	}

	createOpts := hcloud.VolumeCreateOpts{
		Name: req.Name,
		Size: req.SizeGB,
	}
	if req.AttachTo != "" {
		server, _, getErr := s.client.Server.GetByName(ctx, req.AttachTo)
		if getErr != nil {
			return nil, false, "", getErr
		}
		if server == nil {
			return nil, false, "", notFoundError(fmt.Sprintf("instance %q not found", req.AttachTo))
		}
		createOpts.Server = server
	} else {
		location, _, locErr := s.client.Location.GetByName(ctx, req.Region)
		if locErr != nil {
			return nil, false, "", locErr
		}
		if location == nil {
			return nil, false, "", notFoundError(fmt.Sprintf("region %q not found", req.Region))
		}
		createOpts.Location = location
	}

	result, _, err := s.client.Volume.Create(ctx, createOpts)
	if err != nil {
		return nil, false, "", err
	}
	if result.Volume == nil {
		return nil, false, "", fmt.Errorf("hetzner returned empty volume")
	}

	actionID := ""
	if result.Action != nil {
		actionID = fmt.Sprintf("%d", result.Action.ID)
	}
	block := blockStorageFromVolume(result.Volume)
	return &block, true, actionID, nil
}

func (s *RegionService) DeleteBlockStorage(ctx context.Context, name string) (bool, error) {
	if !s.configured {
		return false, ErrNotConfigured
	}
	volume, _, err := s.client.Volume.GetByName(ctx, name)
	if err != nil {
		return false, err
	}
	if volume == nil {
		return false, nil
	}
	_, err = s.client.Volume.Delete(ctx, volume)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *RegionService) AttachBlockStorage(ctx context.Context, name, instanceName string) (bool, string, error) {
	if !s.configured {
		return false, "", ErrNotConfigured
	}
	volume, _, err := s.client.Volume.GetByName(ctx, name)
	if err != nil {
		return false, "", err
	}
	if volume == nil {
		return false, "", nil
	}
	server, _, err := s.client.Server.GetByName(ctx, instanceName)
	if err != nil {
		return false, "", err
	}
	if server == nil {
		return false, "", notFoundError(fmt.Sprintf("instance %q not found", instanceName))
	}
	action, _, err := s.client.Volume.Attach(ctx, volume, server)
	if err != nil {
		return false, "", err
	}
	return true, fmt.Sprintf("%d", action.ID), nil
}

func (s *RegionService) DetachBlockStorage(ctx context.Context, name string) (bool, string, error) {
	if !s.configured {
		return false, "", ErrNotConfigured
	}
	volume, _, err := s.client.Volume.GetByName(ctx, name)
	if err != nil {
		return false, "", err
	}
	if volume == nil {
		return false, "", nil
	}
	action, _, err := s.client.Volume.Detach(ctx, volume)
	if err != nil {
		return false, "", err
	}
	return true, fmt.Sprintf("%d", action.ID), nil
}

func (s *RegionService) getServerByName(ctx context.Context, name string) (*hcloud.Server, error) {
	if !s.configured {
		return nil, ErrNotConfigured
	}
	server, _, err := s.client.Server.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return server, nil
}

func instanceFromServer(server *hcloud.Server) Instance {
	sku := ""
	if server.ServerType != nil {
		sku = strings.ToLower(server.ServerType.Name)
	}
	image := ""
	if server.Image != nil {
		image = strings.ToLower(server.Image.Name)
	}
	region := ""
	if server.Location != nil {
		region = strings.ToLower(server.Location.Name)
	}
	return Instance{
		Name:       strings.ToLower(server.Name),
		SKUName:    sku,
		ImageName:  image,
		Region:     region,
		PowerState: normalizePowerState(server.Status),
		CreatedAt:  server.Created,
	}
}

func blockStorageFromVolume(volume *hcloud.Volume) BlockStorage {
	region := ""
	if volume.Location != nil {
		region = strings.ToLower(volume.Location.Name)
	}
	attachedTo := ""
	if volume.Server != nil {
		attachedTo = strings.ToLower(volume.Server.Name)
	}
	return BlockStorage{
		Name:       strings.ToLower(volume.Name),
		SizeGB:     volume.Size,
		Region:     region,
		AttachedTo: attachedTo,
		CreatedAt:  volume.Created,
	}
}

func normalizePowerState(state hcloud.ServerStatus) string {
	switch state {
	case hcloud.ServerStatusRunning, hcloud.ServerStatusStarting:
		return "on"
	case hcloud.ServerStatusOff, hcloud.ServerStatusStopping:
		return "off"
	default:
		return "off"
	}
}
