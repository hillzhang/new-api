# 分组模型独立倍率（GroupModelRatio）功能 Implementation Plan

## 一、 需求背景与核心价值

当前系统的计费倍率逻辑：
$$\text{有效倍率} = \text{ModelRatio} \times \text{GroupRatio}$$

在实际业务运营中，不同用户分组（如普通用户、VIP、企业客户）对不同模型的需求和利润空间差异巨大。需要支持**在指定分组下为特定模型配置独立折扣倍率**：
- 例如：`vip` 分组的基础倍率为 `0.9`，但对于 `gpt-4o` 给予特惠 `0.8` 倍，而对于 `claude-3-5-sonnet` 不给额外折扣（或保持 `1.0`）；
- `svip` 分组的基础倍率为 `0.8`，对于 `gpt-4o` 给予 `0.5` 倍；
- 未配置独立折扣的模型，自动保持原有倍率（即模型倍率乘以该分组的基础倍率）。

---

## 二、 计费数学模型与计算公式

### 1. 核心计算公式
设：
- 模型基准倍率 / 价格：$\text{ModelRatio}$ 或 $\text{ModelPrice}$
- 分组基础倍率：$\text{GroupRatio}$（若命中用户间特殊分组倍率则为 $\text{GroupSpecialRatio}$）
- 分组下该模型的独立倍率：$\text{GroupModelRatio}$（未单独配置时默认为 $1.0$）

**有效分组倍率（EffectiveGroupRatio）**：
$$\text{EffectiveGroupRatio} = \text{GroupRatio} \times \text{GroupModelRatio}$$

### 2. 各计费模式映射

1. **Token 按量计费（Prompt + Completion + Cache）**：
   $$\text{FinalRatio} = \text{ModelRatio} \times \text{GroupRatio} \times \text{GroupModelRatio}$$
   $$\text{Quota} = (\text{TokensWithRatio}) \times \text{FinalRatio}$$

2. **按次固定价格计费（Per-Call / Midjourney / DALL-E / Task）**：
   $$\text{Quota} = \text{ModelPrice} \times \text{QuotaPerUnit} \times \text{GroupRatio} \times \text{GroupModelRatio}$$

3. **分阶计费（`tiered_expr`）**：
   $$\text{QuotaBeforeGroup} = \text{ExprCost} / 1{,}000{,}000 \times \text{QuotaPerUnit}$$
   $$\text{ActualQuota} = \text{QuotaBeforeGroup} \times \text{GroupRatio} \times \text{GroupModelRatio}$$

4. **独立音频输入与附加费（Tool Surcharge / Audio Input）**：
   $$\text{AudioQuota} = \text{AudioPricePerM} / 1{,}000{,}000 \times \text{AudioTokens} \times \text{GroupRatio} \times \text{GroupModelRatio} \times \text{QuotaPerUnit}$$

---

## 三、 数据存储与模型匹配设计

### 1. 配置数据结构 (JSON)
在全局配置 `GroupRatioSetting` 中新增 `group_model_ratio`：
```json
{
  "default": {
    "gpt-4o-mini": 0.9
  },
  "vip": {
    "gpt-4o": 0.8,
    "claude-3-5-sonnet*": 0.9,
    "*": 1.0
  },
  "svip": {
    "gpt-4o": 0.5,
    "o1*": 0.7
  }
}
```

### 2. 存储与加载机制
- **内存存储**：`types.RWMap[string, map[string]float64]`（分组名 $\rightarrow$ 模型模式 $\rightarrow$ 倍率），线程安全且支持并发读写。
- **持久化**：存储于系统的 `options` 表中，Option Name 为 `GroupModelRatio`。
- **配置同步**：注册到 `setting/config/config.go`，支持服务启动自动加载与后台配置保存时热更新，**无需任何数据库表结构变更 (Zero DB Migration)**。

### 3. 模型匹配算法与优先级
当请求进入时，提取 `UsingGroup` 与 `OriginModelName`：
1. **精确匹配**：查找是否存在 `groupModelRatioMap[UsingGroup][OriginModelName]`，命中则返回该值；
2. **通配符/前缀匹配**：遍历该分组下的通配配置（如 `claude-3-5-*` 或以 `*` 结尾的 pattern），命中则返回该值；
3. **全局缺省**：若该分组下无匹配或未配置该分组，返回 `1.0`。

---

## 四、 具体执行任务清单 (Action Items)

```
new-api/
├── setting/ratio_setting/group_ratio.go     # [Task 1] 配置结构定义、获取与更新函数
├── model/option.go                          # [Task 2] DB Options 映射与持久化绑定
├── types/price_data.go                      # [Task 3] GroupRatioInfo 上下文扩充
├── relay/helper/price.go                    # [Task 4] 预扣费流水线与匹配逻辑
├── pkg/billingexpr/types.go & settle.go     # [Task 5] 分阶计费快照与结算
├── service/text_quota.go                    # [Task 6] 真实扣费结算流水线
├── service/log_info_generate.go             # [Task 7] 消费明细日志记录
└── web/src/features/system-settings/        # [Task 8] 前端设置页 JSON 编辑器集成
```

### Task 1: 配置层支持 (`setting/ratio_setting/group_ratio.go`)
- 在 `GroupRatioSetting` 结构体新增：
  ```go
  GroupModelRatio *types.RWMap[string, map[string]float64] `json:"group_model_ratio"`
  ```
- 定义全局变量 `groupModelRatioMap = types.NewRWMap[string, map[string]float64]()`；
- 实现核心方法：
  - `GetGroupModelRatio(group, modelName string) float64`：执行精确匹配 $\rightarrow$ 通配符匹配 $\rightarrow$ 缺省 `1.0`；
  - `GroupModelRatio2JSONString() string`
  - `UpdateGroupModelRatioByJSONString(jsonStr string) error`
  - `CheckGroupModelRatio(jsonStr string) error`：校验 JSON 语法合法性及倍率非负。

### Task 2: Option 映射与热加载 (`model/option.go`)
- 在 `InitOptionMap()` 中增加默认值映射：
  ```go
  common.OptionMap["GroupModelRatio"] = ratio_setting.GroupModelRatio2JSONString()
  ```
- 在 `UpdateOption()` 的 switch 分支增加：
  ```go
  case "GroupModelRatio":
      err = ratio_setting.UpdateGroupModelRatioByJSONString(value)
  ```

### Task 3: 计费上下文结构扩充 (`types/price_data.go`)
- 在 `GroupRatioInfo` 中增加字段与便捷方法：
  ```go
  type GroupRatioInfo struct {
      GroupRatio        float64
      GroupSpecialRatio float64
      HasSpecialRatio   bool
      GroupModelRatio   float64 // 分组下特定模型的折扣倍率（默认 1.0）
  }

  func (g GroupRatioInfo) EffectiveRatio() float64 {
      if g.GroupModelRatio > 0 {
          return g.GroupRatio * g.GroupModelRatio
      }
      return g.GroupRatio
  }
  ```

### Task 4: 预扣费流水线接入 (`relay/helper/price.go`)
- 在 `HandleGroupRatio(ctx, relayInfo)` 中：
  ```go
  groupModelRatio := ratio_setting.GetGroupModelRatio(relayInfo.UsingGroup, relayInfo.OriginModelName)
  groupRatioInfo.GroupModelRatio = groupModelRatio
  ```
- 更新 `ModelPriceHelper`：
  - 按量计费预扣 `preConsumedQuota` 计算时，采用 `groupRatioInfo.EffectiveRatio()`；
  - 按固定价格预扣时，计算 `modelPrice * common.QuotaPerUnit * groupRatioInfo.EffectiveRatio()`。
- 更新 `ModelPriceHelperPerCall`：
  - 按次计费的 `quota` 计算统一采用 `groupRatioInfo.EffectiveRatio()`。
- 更新 `modelPriceHelperTiered`：
  - 阶梯计费预扣额度计算乘以 `groupRatioInfo.EffectiveRatio()`；
  - 将 `GroupModelRatio` 存入 `BillingSnapshot` 中。

### Task 5: 分阶计费快照与结算接入 (`pkg/billingexpr/`)
- 在 `pkg/billingexpr/types.go` 的 `BillingSnapshot` 增加字段：
  ```go
  GroupModelRatio float64 `json:"group_model_ratio"`
  ```
- 在 `pkg/billingexpr/settle.go` 的 `ComputeTieredQuotaWithRequest` 中：
  ```go
  effectiveGroupRatio := snap.GroupRatio
  if snap.GroupModelRatio > 0 {
      effectiveGroupRatio *= snap.GroupModelRatio
  }
  afterGroup, clamp := common.QuotaRoundChecked(quotaBeforeGroup * effectiveGroupRatio)
  ```

### Task 6: 真实扣费结算流水线 (`service/text_quota.go`)
- 在 `textQuotaSummary` 结构体中添加 `GroupModelRatio float64`；
- 在 `calculateTextQuotaSummary` 中：
  ```go
  summary.GroupModelRatio = relayInfo.PriceData.GroupRatioInfo.GroupModelRatio
  if summary.GroupModelRatio <= 0 {
      summary.GroupModelRatio = 1.0
  }
  ```
- 扣费计算：
  - `dGroupModelRatio := decimal.NewFromFloat(summary.GroupModelRatio)`
  - `ratio := dModelRatio.Mul(dGroupRatio).Mul(dGroupModelRatio)`
  - 按次计算：`quotaCalculateDecimal := dModelPrice.Mul(dQuotaPerUnit).Mul(dGroupRatio).Mul(dGroupModelRatio)`
  - 独立音频：`audioInputQuota.Mul(dGroupModelRatio)`
  - Tool Surcharge：同样乘上 `dGroupModelRatio` 保证一致性。

### Task 7: 审计与消费日志记录 (`service/log_info_generate.go` & `service/text_quota.go`)
- 在 `GenerateTextOtherInfo` 和 `GenerateClaudeOtherInfo` 中，将 `group_model_ratio` 记录到日志 `other` 字典：
  ```go
  if summary.GroupModelRatio != 1.0 {
      other["group_model_ratio"] = summary.GroupModelRatio
  }
  ```
- 管理员可在「日志」界面清晰查看单次请求实际应用的基准倍率、分组倍率和模型独立倍率。

### Task 8: 前端管理界面集成 (`web/src/features/system-settings/`)
- 在 `types.ts` 中的 `GroupFormValues` 增加 `GroupModelRatio: string`；
- 在 `group-ratio-form.tsx` 中添加 `GroupModelRatio` 的 JSON 编辑器（利用已有的 `JsonCodeEditor` 组件），提供语法提示与占位符示例；
- 在保存设置时自动提交并做 JSON 有效性验证。

---

## 五、 测试与验证方案

### 1. 单元测试覆盖
- [ ] `setting/ratio_setting/group_ratio_test.go`：
  - 测试未设置任何 `GroupModelRatio` 时的默认返回 `1.0`；
  - 测试精确匹配（如 `vip` 下 `gpt-4o` 返回 `0.8`）；
  - 测试前缀通配符匹配（如 `svip` 下 `claude-*` 返回 `0.85`）；
  - 测试非法 JSON 输入校验与拦截。
- [ ] `relay/helper/price_test.go`：
  - 测试按量计费预扣额度：验证 `GroupRatio * GroupModelRatio` 计算准确；
  - 测试按次计费预扣额度：验证 `ModelPrice * GroupRatio * GroupModelRatio`。
- [ ] `service/text_quota_test.go` 与 `service/tiered_settle_test.go`：
  - 验证最终完成请求扣费、阶梯计费结算的金额与日志字段 `other["group_model_ratio"]`。

### 2. 回归与兼容性验证
- [ ] **向后兼容验证**：对于老配置（数据库中未配置 `GroupModelRatio` 或留空），所有模型计费结果与改动前 100% 一致；
- [ ] **多模型并发验证**：在同一分组下并发调用配置了独立倍率和未配置独立倍率的不同模型，确保倍率隔离互不影响。
