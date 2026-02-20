package httpserver

import "sync"

type resourceRuntimeState struct {
	mu                sync.RWMutex
	instanceSpecs     map[string]instanceSpec
	blockStorageSpecs map[string]blockStorageSpec
	images            map[string]imageRuntimeRecord
	networks          map[string]networkRuntimeRecord
	internetGateways  map[string]internetGatewayRuntimeRecord
	routeTables       map[string]routeTableRuntimeRecord
	subnets           map[string]subnetRuntimeRecord
	publicIPs         map[string]publicIPRuntimeRecord
	nics              map[string]nicRuntimeRecord
	securityGroups    map[string]securityGroupRuntimeRecord
}

var runtimeResourceState = &resourceRuntimeState{
	instanceSpecs:     map[string]instanceSpec{},
	blockStorageSpecs: map[string]blockStorageSpec{},
	images:            map[string]imageRuntimeRecord{},
	networks:          map[string]networkRuntimeRecord{},
	internetGateways:  map[string]internetGatewayRuntimeRecord{},
	routeTables:       map[string]routeTableRuntimeRecord{},
	subnets:           map[string]subnetRuntimeRecord{},
	publicIPs:         map[string]publicIPRuntimeRecord{},
	nics:              map[string]nicRuntimeRecord{},
	securityGroups:    map[string]securityGroupRuntimeRecord{},
}

type imageRuntimeRecord struct {
	Tenant          string
	Name            string
	Region          string
	Labels          map[string]string
	Spec            imageSpec
	CreatedAt       string
	LastModifiedAt  string
	ResourceVersion int64
}

type networkRuntimeRecord struct {
	Tenant          string
	Workspace       string
	Name            string
	Region          string
	Labels          map[string]string
	Spec            networkSpec
	CreatedAt       string
	LastModifiedAt  string
	ResourceVersion int64
}

type internetGatewayRuntimeRecord struct {
	Tenant          string
	Workspace       string
	Name            string
	Region          string
	Labels          map[string]string
	Spec            internetGatewaySpec
	CreatedAt       string
	LastModifiedAt  string
	ResourceVersion int64
}

type routeTableRuntimeRecord struct {
	Tenant          string
	Workspace       string
	Network         string
	Name            string
	Region          string
	Labels          map[string]string
	Spec            routeTableSpec
	CreatedAt       string
	LastModifiedAt  string
	ResourceVersion int64
}

type subnetRuntimeRecord struct {
	Tenant          string
	Workspace       string
	Network         string
	Name            string
	Region          string
	Labels          map[string]string
	Spec            subnetSpec
	CreatedAt       string
	LastModifiedAt  string
	ResourceVersion int64
}

type publicIPRuntimeRecord struct {
	Tenant          string
	Workspace       string
	Name            string
	Region          string
	Labels          map[string]string
	Spec            publicIPSpec
	CreatedAt       string
	LastModifiedAt  string
	ResourceVersion int64
}

type nicRuntimeRecord struct {
	Tenant          string
	Workspace       string
	Name            string
	Region          string
	Labels          map[string]string
	Spec            nicSpec
	CreatedAt       string
	LastModifiedAt  string
	ResourceVersion int64
}

type securityGroupRuntimeRecord struct {
	Tenant          string
	Workspace       string
	Name            string
	Region          string
	Labels          map[string]string
	Spec            securityGroupSpec
	CreatedAt       string
	LastModifiedAt  string
	ResourceVersion int64
}

func (s *resourceRuntimeState) setInstanceSpec(key string, spec instanceSpec) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.instanceSpecs[key] = spec
}

func (s *resourceRuntimeState) getInstanceSpec(key string) (instanceSpec, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	spec, ok := s.instanceSpecs[key]
	return spec, ok
}

func (s *resourceRuntimeState) deleteInstanceSpec(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.instanceSpecs, key)
}

func (s *resourceRuntimeState) setBlockStorageSpec(key string, spec blockStorageSpec) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blockStorageSpecs[key] = spec
}

func (s *resourceRuntimeState) getBlockStorageSpec(key string) (blockStorageSpec, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	spec, ok := s.blockStorageSpecs[key]
	return spec, ok
}

func (s *resourceRuntimeState) deleteBlockStorageSpec(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.blockStorageSpecs, key)
}

func (s *resourceRuntimeState) upsertImage(key string, rec imageRuntimeRecord) (imageRuntimeRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.images[key]
	if ok {
		rec.CreatedAt = existing.CreatedAt
		rec.ResourceVersion = existing.ResourceVersion + 1
	} else {
		rec.ResourceVersion = 1
	}
	s.images[key] = rec
	return rec, !ok
}

func (s *resourceRuntimeState) getImage(key string) (imageRuntimeRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.images[key]
	return rec, ok
}

func (s *resourceRuntimeState) listImagesByTenant(tenant string) []imageRuntimeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]imageRuntimeRecord, 0, len(s.images))
	for _, rec := range s.images {
		if rec.Tenant == tenant {
			out = append(out, rec)
		}
	}
	return out
}

func (s *resourceRuntimeState) deleteImage(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.images, key)
}

func (s *resourceRuntimeState) upsertNetwork(key string, rec networkRuntimeRecord) (networkRuntimeRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.networks[key]
	if ok {
		rec.CreatedAt = existing.CreatedAt
		rec.ResourceVersion = existing.ResourceVersion + 1
	} else {
		rec.ResourceVersion = 1
	}
	s.networks[key] = rec
	return rec, !ok
}

func (s *resourceRuntimeState) getNetwork(key string) (networkRuntimeRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.networks[key]
	return rec, ok
}

func (s *resourceRuntimeState) listNetworksByScope(tenant, workspace string) []networkRuntimeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]networkRuntimeRecord, 0, len(s.networks))
	for _, rec := range s.networks {
		if rec.Tenant == tenant && rec.Workspace == workspace {
			out = append(out, rec)
		}
	}
	return out
}

func (s *resourceRuntimeState) deleteNetwork(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.networks, key)
}

func (s *resourceRuntimeState) upsertInternetGateway(key string, rec internetGatewayRuntimeRecord) (internetGatewayRuntimeRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.internetGateways[key]
	if ok {
		rec.CreatedAt = existing.CreatedAt
		rec.ResourceVersion = existing.ResourceVersion + 1
	} else {
		rec.ResourceVersion = 1
	}
	s.internetGateways[key] = rec
	return rec, !ok
}

func (s *resourceRuntimeState) getInternetGateway(key string) (internetGatewayRuntimeRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.internetGateways[key]
	return rec, ok
}

func (s *resourceRuntimeState) listInternetGatewaysByScope(tenant, workspace string) []internetGatewayRuntimeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]internetGatewayRuntimeRecord, 0, len(s.internetGateways))
	for _, rec := range s.internetGateways {
		if rec.Tenant == tenant && rec.Workspace == workspace {
			out = append(out, rec)
		}
	}
	return out
}

func (s *resourceRuntimeState) deleteInternetGateway(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.internetGateways, key)
}

func (s *resourceRuntimeState) upsertRouteTable(key string, rec routeTableRuntimeRecord) (routeTableRuntimeRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.routeTables[key]
	if ok {
		rec.CreatedAt = existing.CreatedAt
		rec.ResourceVersion = existing.ResourceVersion + 1
	} else {
		rec.ResourceVersion = 1
	}
	s.routeTables[key] = rec
	return rec, !ok
}

func (s *resourceRuntimeState) getRouteTable(key string) (routeTableRuntimeRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.routeTables[key]
	return rec, ok
}

func (s *resourceRuntimeState) listRouteTablesByScope(tenant, workspace, network string) []routeTableRuntimeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]routeTableRuntimeRecord, 0, len(s.routeTables))
	for _, rec := range s.routeTables {
		if rec.Tenant == tenant && rec.Workspace == workspace && rec.Network == network {
			out = append(out, rec)
		}
	}
	return out
}

func (s *resourceRuntimeState) deleteRouteTable(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.routeTables, key)
}

func (s *resourceRuntimeState) upsertSubnet(key string, rec subnetRuntimeRecord) (subnetRuntimeRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.subnets[key]
	if ok {
		rec.CreatedAt = existing.CreatedAt
		rec.ResourceVersion = existing.ResourceVersion + 1
	} else {
		rec.ResourceVersion = 1
	}
	s.subnets[key] = rec
	return rec, !ok
}

func (s *resourceRuntimeState) getSubnet(key string) (subnetRuntimeRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.subnets[key]
	return rec, ok
}

func (s *resourceRuntimeState) listSubnetsByScope(tenant, workspace, network string) []subnetRuntimeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]subnetRuntimeRecord, 0, len(s.subnets))
	for _, rec := range s.subnets {
		if rec.Tenant == tenant && rec.Workspace == workspace && rec.Network == network {
			out = append(out, rec)
		}
	}
	return out
}

func (s *resourceRuntimeState) deleteSubnet(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subnets, key)
}

func (s *resourceRuntimeState) upsertPublicIP(key string, rec publicIPRuntimeRecord) (publicIPRuntimeRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.publicIPs[key]
	if ok {
		rec.CreatedAt = existing.CreatedAt
		rec.ResourceVersion = existing.ResourceVersion + 1
	} else {
		rec.ResourceVersion = 1
	}
	s.publicIPs[key] = rec
	return rec, !ok
}

func (s *resourceRuntimeState) getPublicIP(key string) (publicIPRuntimeRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.publicIPs[key]
	return rec, ok
}

func (s *resourceRuntimeState) listPublicIPsByScope(tenant, workspace string) []publicIPRuntimeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]publicIPRuntimeRecord, 0, len(s.publicIPs))
	for _, rec := range s.publicIPs {
		if rec.Tenant == tenant && rec.Workspace == workspace {
			out = append(out, rec)
		}
	}
	return out
}

func (s *resourceRuntimeState) deletePublicIP(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.publicIPs, key)
}

func (s *resourceRuntimeState) upsertNIC(key string, rec nicRuntimeRecord) (nicRuntimeRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.nics[key]
	if ok {
		rec.CreatedAt = existing.CreatedAt
		rec.ResourceVersion = existing.ResourceVersion + 1
	} else {
		rec.ResourceVersion = 1
	}
	s.nics[key] = rec
	return rec, !ok
}

func (s *resourceRuntimeState) getNIC(key string) (nicRuntimeRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.nics[key]
	return rec, ok
}

func (s *resourceRuntimeState) listNICsByScope(tenant, workspace string) []nicRuntimeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]nicRuntimeRecord, 0, len(s.nics))
	for _, rec := range s.nics {
		if rec.Tenant == tenant && rec.Workspace == workspace {
			out = append(out, rec)
		}
	}
	return out
}

func (s *resourceRuntimeState) deleteNIC(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nics, key)
}

func (s *resourceRuntimeState) upsertSecurityGroup(key string, rec securityGroupRuntimeRecord) (securityGroupRuntimeRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.securityGroups[key]
	if ok {
		rec.CreatedAt = existing.CreatedAt
		rec.ResourceVersion = existing.ResourceVersion + 1
	} else {
		rec.ResourceVersion = 1
	}
	s.securityGroups[key] = rec
	return rec, !ok
}

func (s *resourceRuntimeState) getSecurityGroup(key string) (securityGroupRuntimeRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.securityGroups[key]
	return rec, ok
}

func (s *resourceRuntimeState) listSecurityGroupsByScope(tenant, workspace string) []securityGroupRuntimeRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]securityGroupRuntimeRecord, 0, len(s.securityGroups))
	for _, rec := range s.securityGroups {
		if rec.Tenant == tenant && rec.Workspace == workspace {
			out = append(out, rec)
		}
	}
	return out
}

func (s *resourceRuntimeState) deleteSecurityGroup(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.securityGroups, key)
}
