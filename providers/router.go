package providers

import (
	"context"
	"fmt"
	"sync"
)

// Router routes Chat requests to named providers.
type Router struct {
	mu        sync.RWMutex
	providers map[string]Provider
	fallback  string
}

// NewRouter returns a router with the given default provider name (e.g. "deepseek").
func NewRouter(fallback string) *Router {
	return &Router{
		providers: make(map[string]Provider),
		fallback:  fallback,
	}
}

// Register adds a provider (name from p.Name()).
func (r *Router) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

// Chat routes to a provider based on req.Model (see parseModel).
func (r *Router) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providerName, model := r.parseModel(req.Model)
	p, ok := r.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", providerName)
	}
	if !p.Available() {
		return nil, fmt.Errorf("provider %q not available", providerName)
	}

	reqCopy := *req
	reqCopy.Model = model
	return p.Chat(ctx, &reqCopy)
}

// ChatStream routes streaming to the resolved provider.
func (r *Router) ChatStream(ctx context.Context, req *ChatRequest) (<-chan string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providerName, model := r.parseModel(req.Model)
	p, ok := r.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", providerName)
	}
	if !p.Available() {
		return nil, fmt.Errorf("provider %q not available", providerName)
	}

	reqCopy := *req
	reqCopy.Model = model
	return p.ChatStream(ctx, &reqCopy)
}

func (r *Router) parseModel(model string) (providerName, modelName string) {
	if model == "" {
		return r.fallback, ""
	}
	for i, c := range model {
		if c == ':' {
			return model[:i], model[i+1:]
		}
	}
	if _, ok := r.providers[model]; ok {
		return model, ""
	}
	return r.fallback, model
}

// ListProviders returns registered providers that are Available().
func (r *Router) ListProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var names []string
	for name, p := range r.providers {
		if p.Available() {
			names = append(names, name)
		}
	}
	return names
}
