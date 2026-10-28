package ratio_setting

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

var defaultGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

var groupRatioMap = types.NewRWMap[string, float64]()

var defaultGroupGroupRatio = map[string]map[string]float64{
	"vip": {
		"edit_this": 0.9,
	},
}

var groupGroupRatioMap = types.NewRWMap[string, map[string]float64]()

var defaultGroupModelRatio = map[string]map[string]float64{}

var groupModelRatioMap = types.NewRWMap[string, map[string]float64]()

var defaultGroupSpecialUsableGroup = map[string]map[string]string{}

type GroupRatioSetting struct {
	GroupRatio              *types.RWMap[string, float64]            `json:"group_ratio"`
	GroupGroupRatio         *types.RWMap[string, map[string]float64] `json:"group_group_ratio"`
	GroupModelRatio         *types.RWMap[string, map[string]float64] `json:"group_model_ratio"`
	GroupSpecialUsableGroup *types.RWMap[string, map[string]string]  `json:"group_special_usable_group"`
}

var groupRatioSetting GroupRatioSetting

func init() {
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)

	groupRatioMap.AddAll(defaultGroupRatio)
	groupGroupRatioMap.AddAll(defaultGroupGroupRatio)
	groupModelRatioMap.AddAll(defaultGroupModelRatio)

	groupRatioSetting = GroupRatioSetting{
		GroupSpecialUsableGroup: groupSpecialUsableGroup,
		GroupRatio:              groupRatioMap,
		GroupGroupRatio:         groupGroupRatioMap,
		GroupModelRatio:         groupModelRatioMap,
	}

	config.GlobalConfig.Register("group_ratio_setting", &groupRatioSetting)
}

func GetGroupRatioSetting() *GroupRatioSetting {
	if groupRatioSetting.GroupSpecialUsableGroup == nil {
		groupRatioSetting.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
		groupRatioSetting.GroupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)
	}
	if groupRatioSetting.GroupModelRatio == nil {
		groupRatioSetting.GroupModelRatio = groupModelRatioMap
	}
	return &groupRatioSetting
}

func GetGroupRatioCopy() map[string]float64 {
	return groupRatioMap.ReadAll()
}

func ContainsGroupRatio(name string) bool {
	_, ok := groupRatioMap.Get(name)
	return ok
}

func GroupRatio2JSONString() string {
	return groupRatioMap.MarshalJSONString()
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupRatioMap, jsonStr)
}

func GetGroupRatio(name string) float64 {
	ratio, ok := groupRatioMap.Get(name)
	if !ok {
		common.SysLog("group ratio not found: " + name)
		return 1
	}
	return ratio
}

func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	gp, ok := groupGroupRatioMap.Get(userGroup)
	if !ok {
		return -1, false
	}
	ratio, ok := gp[usingGroup]
	if !ok {
		return -1, false
	}
	return ratio, true
}

func GroupGroupRatio2JSONString() string {
	return groupGroupRatioMap.MarshalJSONString()
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupGroupRatioMap, jsonStr)
}

func GroupModelRatio2JSONString() string {
	return groupModelRatioMap.MarshalJSONString()
}

func UpdateGroupModelRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupModelRatioMap, jsonStr)
}

// GetGroupModelRatio returns the model ratio configured under the given group.
// It prioritizes exact match, then longest-prefix wildcard match (e.g. "gpt-4*"), then "*".
// If not found, it returns 1.0.
func GetGroupModelRatio(group, modelName string) float64 {
	groupModels, ok := groupModelRatioMap.Get(group)
	if !ok || len(groupModels) == 0 {
		return 1.0
	}

	// 1. Exact match
	if ratio, found := groupModels[modelName]; found {
		return ratio
	}

	// 2. Wildcard matching
	var wildcardMatch float64
	wildcardFound := false
	longestMatchLen := -1

	for pattern, ratio := range groupModels {
		if pattern == "*" {
			if longestMatchLen < 0 {
				wildcardMatch = ratio
				wildcardFound = true
			}
			continue
		}
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(modelName, prefix) && len(prefix) > longestMatchLen {
				longestMatchLen = len(prefix)
				wildcardMatch = ratio
				wildcardFound = true
			}
		}
	}

	if wildcardFound {
		return wildcardMatch
	}

	return 1.0
}

func CheckGroupRatio(jsonStr string) error {
	checkGroupRatio := make(map[string]float64)
	err := json.Unmarshal([]byte(jsonStr), &checkGroupRatio)
	if err != nil {
		return err
	}
	for name, ratio := range checkGroupRatio {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + name)
		}
	}
	return nil
}

func CheckGroupModelRatio(jsonStr string) error {
	checkGroupModelRatio := make(map[string]map[string]float64)
	err := json.Unmarshal([]byte(jsonStr), &checkGroupModelRatio)
	if err != nil {
		return err
	}
	for group, models := range checkGroupModelRatio {
		for modelName, ratio := range models {
			if ratio < 0 {
				return errors.New("group model ratio must be not less than 0: " + group + " -> " + modelName)
			}
		}
	}
	return nil
}
