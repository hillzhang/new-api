package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskBillingContext_EffectiveGroupRatio(t *testing.T) {
	t.Run("nil context returns 1.0", func(t *testing.T) {
		var bc *TaskBillingContext
		assert.Equal(t, 1.0, bc.EffectiveGroupRatio())
	})

	t.Run("base group ratio only", func(t *testing.T) {
		bc := &TaskBillingContext{
			GroupRatio: 2.0,
		}
		assert.Equal(t, 2.0, bc.EffectiveGroupRatio())
	})

	t.Run("with model ratio configured", func(t *testing.T) {
		bc := &TaskBillingContext{
			GroupRatio:         2.0,
			GroupModelRatio:    0.5,
			HasGroupModelRatio: true,
		}
		assert.Equal(t, 1.0, bc.EffectiveGroupRatio())
	})

	t.Run("free model ratio (0.0)", func(t *testing.T) {
		bc := &TaskBillingContext{
			GroupRatio:         2.0,
			GroupModelRatio:    0.0,
			HasGroupModelRatio: true,
		}
		assert.Equal(t, 0.0, bc.EffectiveGroupRatio())
	})

	t.Run("unflagged positive model ratio", func(t *testing.T) {
		bc := &TaskBillingContext{
			GroupRatio:         1.5,
			GroupModelRatio:    0.8,
			HasGroupModelRatio: false,
		}
		assert.InDelta(t, 1.2, bc.EffectiveGroupRatio(), 1e-9)
	})
}
