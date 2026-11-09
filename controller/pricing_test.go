package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pricingResponse struct {
	Success         bool                           `json:"success"`
	GroupRatio      map[string]float64             `json:"group_ratio"`
	GroupModelRatio map[string]map[string]float64 `json:"group_model_ratio"`
	UsableGroup     map[string]string              `json:"usable_group"`
}

func TestGetPricing_GroupModelRatioFilteredByUsableGroup(t *testing.T) {
	setupModelListControllerTestDB(t)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default": 1.0, "vip": 0.8, "secret": 0.5}`))
	require.NoError(t, ratio_setting.UpdateGroupModelRatioByJSONString(`{
		"default": {"gpt-4o": 0.9},
		"vip": {"gpt-4o": 0.8},
		"secret": {"secret-model": 0.1}
	}`))

	// Create user belonging to "vip" group, who can only access "vip" and "default"
	user := &model.User{
		Username: "pricing_user",
		Group:    "vip",
		Role:     common.RoleCommonUser,
	}
	require.NoError(t, model.DB.Create(user).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/pricing", nil)
	c.Set("id", user.Id)

	GetPricing(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp pricingResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)

	// secret group should NOT be present in group_model_ratio because user has no access to it
	assert.NotContains(t, resp.GroupModelRatio, "secret")
	assert.NotContains(t, resp.GroupRatio, "secret")

	// Accessible groups should have their group_model_ratio intact
	if _, ok := resp.UsableGroup["vip"]; ok {
		assert.Contains(t, resp.GroupModelRatio, "vip")
		assert.Equal(t, 0.8, resp.GroupModelRatio["vip"]["gpt-4o"])
	}
	if _, ok := resp.UsableGroup["default"]; ok {
		assert.Contains(t, resp.GroupModelRatio, "default")
		assert.Equal(t, 0.9, resp.GroupModelRatio["default"]["gpt-4o"])
	}
}
