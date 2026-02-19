package main

import (
	"sync"
	"time"
)

// ServiceStatusCache caches service statuses to reduce SCM query frequency
type ServiceStatusCache struct {
	cache map[string]*CachedServiceStatus
	mutex sync.RWMutex
	ttl   time.Duration
}

// CachedServiceStatus represents a cached service status
type CachedServiceStatus struct {
	Status    string
	PID       int
	Timestamp time.Time
}

// NewServiceStatusCache creates a new service status cache
func NewServiceStatusCache() *ServiceStatusCache {
	return &ServiceStatusCache{
		cache: make(map[string]*CachedServiceStatus),
		ttl:   5 * time.Second, // cache TTL: 5 seconds
	}
}

// Get retrieves a cached service status
func (cache *ServiceStatusCache) Get(serviceName string) (*CachedServiceStatus, bool) {
	cache.mutex.RLock()
	defer cache.mutex.RUnlock()

	status, exists := cache.cache[serviceName]
	if !exists {
		return nil, false
	}

	if time.Since(status.Timestamp) > cache.ttl {
		return nil, false
	}

	return status, true
}

// Set stores a service status in the cache
func (cache *ServiceStatusCache) Set(serviceName string, status string, pid int) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	cache.cache[serviceName] = &CachedServiceStatus{
		Status:    status,
		PID:       pid,
		Timestamp: time.Now(),
	}
}

// Remove deletes a service status from the cache
func (cache *ServiceStatusCache) Remove(serviceName string) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	delete(cache.cache, serviceName)
}

// Clear empties the entire cache
func (cache *ServiceStatusCache) Clear() {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	cache.cache = make(map[string]*CachedServiceStatus)
}

// CleanExpired removes expired cache entries
func (cache *ServiceStatusCache) CleanExpired() {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	now := time.Now()
	for serviceName, status := range cache.cache {
		if now.Sub(status.Timestamp) > cache.ttl {
			delete(cache.cache, serviceName)
		}
	}
}

// StartCleanupRoutine starts a goroutine that periodically cleans expired cache entries
func (cache *ServiceStatusCache) StartCleanupRoutine() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				cache.CleanExpired()
			}
		}
	}()
}