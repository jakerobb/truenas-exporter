package truenas

import (
	"context"
	"fmt"
)

// Pool is the subset of pool.query fields the exporter uses. Size fields are
// raw vdev capacity (parity included) and are null while a pool is offline.
type Pool struct {
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	Healthy   bool    `json:"healthy"`
	Size      *uint64 `json:"size"`
	Allocated *uint64 `json:"allocated"`
	Free      *uint64 `json:"free"`
}

// PoolUsage is a pool's usable space, from its root dataset. This is what
// the TrueNAS UI reports as used/available (parity and overhead excluded).
type PoolUsage struct {
	Name      string
	Used      uint64
	Available uint64
}

type zfsProperty struct {
	Parsed *uint64 `json:"parsed"`
}

type dataset struct {
	Name      string      `json:"name"`
	Used      zfsProperty `json:"used"`
	Available zfsProperty `json:"available"`
}

// Pools returns all pools.
func (c *Client) Pools(ctx context.Context) ([]Pool, error) {
	var pools []Pool
	if err := c.Call(ctx, "pool.query", nil, &pools); err != nil {
		return nil, err
	}
	return pools, nil
}

// PoolUsages returns usable space for the named pools' root datasets.
func (c *Client) PoolUsages(ctx context.Context, poolNames []string) ([]PoolUsage, error) {
	if len(poolNames) == 0 {
		return nil, nil
	}
	filters := []any{[]any{"name", "in", poolNames}}
	options := map[string]any{
		"extra": map[string]any{
			"flat":              true,
			"retrieve_children": false,
			"properties":        []string{"used", "available"},
		},
	}
	var datasets []dataset
	if err := c.Call(ctx, "pool.dataset.query", []any{filters, options}, &datasets); err != nil {
		return nil, err
	}

	usages := make([]PoolUsage, 0, len(datasets))
	for _, d := range datasets {
		if d.Used.Parsed == nil || d.Available.Parsed == nil {
			return nil, fmt.Errorf("dataset %s: missing used/available", d.Name)
		}
		usages = append(usages, PoolUsage{Name: d.Name, Used: *d.Used.Parsed, Available: *d.Available.Parsed})
	}
	return usages, nil
}
