package memory

import (
	"context"
	"sync"

	"github.com/elug3/dupli1/payment/pkg/domain"
)

// KillSwitches is an in-process ports.KillSwitchStore. Like the in-memory
// payment repository it is only right for a single replica: a switch flipped
// on one process is not seen by another.
type KillSwitches struct {
	mu       sync.RWMutex
	switches map[string]domain.KillSwitch
}

func NewKillSwitches() *KillSwitches {
	return &KillSwitches{switches: make(map[string]domain.KillSwitch)}
}

func (k *KillSwitches) ListKillSwitches(ctx context.Context) ([]domain.KillSwitch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	k.mu.RLock()
	defer k.mu.RUnlock()
	out := make([]domain.KillSwitch, 0, len(k.switches))
	for _, sw := range k.switches {
		out = append(out, sw)
	}
	return out, nil
}

func (k *KillSwitches) SaveKillSwitch(ctx context.Context, sw domain.KillSwitch) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.switches[sw.Key] = sw
	return nil
}
