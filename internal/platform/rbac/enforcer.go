package rbac

import (
	"fmt"

	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"

	"github.com/your-org/go-template/internal/repository"
)

// NewEnforcer builds the Casbin enforcer over the policy store and loads
// the persisted rules. Autosave is on: every permission change through the
// enforcer is persisted through the adapter in the same call.
func NewEnforcer(store repository.RuleStore) (*casbin.SyncedEnforcer, error) {
	m, err := casbinmodel.NewModelFromString(ModelText)
	if err != nil {
		return nil, fmt.Errorf("rbac model: %w", err)
	}
	e, err := casbin.NewSyncedEnforcer(m, NewAdapter(store))
	if err != nil {
		return nil, fmt.Errorf("rbac enforcer: %w", err)
	}
	if err := e.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("rbac load policy: %w", err)
	}
	return e, nil
}
