package workers

import (
	"context"
	"sync"
)

// Registry holds all registered per-adapter crawler services and starts them concurrently.
type Registry struct {
	services []AdapterCrawlerService
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a crawler service to the registry.
func (r *Registry) Register(svc AdapterCrawlerService) {
	r.services = append(r.services, svc)
}

// Len returns the number of registered crawler services.
func (r *Registry) Len() int {
	return len(r.services)
}

// StartAll launches each registered service in its own goroutine.
// Each goroutine calls wg.Done when its Run method returns (i.e. when ctx is cancelled).
func (r *Registry) StartAll(ctx context.Context, wg *sync.WaitGroup) {
	for _, svc := range r.services {
		wg.Add(1)
		go func(s AdapterCrawlerService) {
			defer wg.Done()
			s.Run(ctx)
		}(svc)
	}
}
