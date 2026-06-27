package strategy_test

import (
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/strategy"
	"github.com/stretchr/testify/assert"
)

func TestResolveStrategy(t *testing.T) {
	assert.Equal(t, model.StrategyImmediate, strategy.Resolve("production", "rolling", false, true))
	assert.Equal(t, model.StrategyPreview, strategy.Resolve("preview", "rolling", false, false))
	assert.Equal(t, model.StrategyPreview, strategy.Resolve("production", "rolling", true, false))
	assert.Equal(t, model.StrategyBlueGreen, strategy.Resolve("production", model.StrategyBlueGreen, false, false))
	assert.Equal(t, model.StrategyRolling, strategy.Resolve("production", "", false, false))
}

func TestRegistryGet(t *testing.T) {
	reg := strategy.NewRegistry(model.StrategyRolling)
	assert.NotNil(t, reg.Get(model.StrategyCanary))
	assert.NotNil(t, reg.Get("unknown"))
}
