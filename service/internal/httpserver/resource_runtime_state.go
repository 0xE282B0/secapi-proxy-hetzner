package httpserver

import "sync"

type resourceRuntimeState struct {
	mu                sync.RWMutex
	instanceSpecs     map[string]instanceSpec
	blockStorageSpecs map[string]blockStorageSpec
}

var runtimeResourceState = &resourceRuntimeState{
	instanceSpecs:     map[string]instanceSpec{},
	blockStorageSpecs: map[string]blockStorageSpec{},
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
