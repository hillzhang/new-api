package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetGroupModelRatio(t *testing.T) {
	testJSON := `{
		"vip": {
			"gpt-4o": 0.8,
			"claude-3-5*": 0.9,
			"*": 0.95
		},
		"svip": {
			"gpt-4o": 0.5
		}
	}`

	err := UpdateGroupModelRatioByJSONString(testJSON)
	assert.NoError(t, err)

	// 1. Exact match
	assert.Equal(t, 0.8, GetGroupModelRatio("vip", "gpt-4o"))
	assert.Equal(t, 0.5, GetGroupModelRatio("svip", "gpt-4o"))

	// 2. Longest prefix wildcard match
	assert.Equal(t, 0.9, GetGroupModelRatio("vip", "claude-3-5-sonnet-20241022"))

	// 3. Universal wildcard match
	assert.Equal(t, 0.95, GetGroupModelRatio("vip", "deepseek-chat"))

	// 4. Configured group but model not configured and no wildcard in that group
	assert.Equal(t, 1.0, GetGroupModelRatio("svip", "claude-3-5-sonnet"))

	// 5. Completely unconfigured group
	assert.Equal(t, 1.0, GetGroupModelRatio("normal", "gpt-4o"))
}

func TestCheckGroupModelRatio(t *testing.T) {
	validJSON := `{"vip": {"gpt-4o": 0.8}}`
	assert.NoError(t, CheckGroupModelRatio(validJSON))

	negativeRatioJSON := `{"vip": {"gpt-4o": -0.5}}`
	assert.Error(t, CheckGroupModelRatio(negativeRatioJSON))

	invalidJSON := `{"vip": invalid}`
	assert.Error(t, CheckGroupModelRatio(invalidJSON))
}
