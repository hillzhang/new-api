# MiniMax-H3 模型接入 Implementation Plan (V3 优化版)

> **修订说明**：
> 1. **ModelPrice 严格统一为“每秒基准单价（元/秒）”**：废弃原先按 5 秒打包的旧方案，严格对齐 New API 视频模型标准（如 Sora、Veo 等统一以每秒单价作为 `ModelPrice`，时长通过 `OtherRatios["seconds"]` 直接体现）；
> 2. **`MiniMax-H3-Max` 默认基准规格设为 480P**：默认空键 `{}` 对应 480P（0.33 元/秒），升级 768P 为上浮倍率，彻底消除 `< 1.0` 的折扣倒挂。

---

## 一、 官方定价与 New API 计费项精确映射表（纯人民币）

所有模型的 `ModelPrice` 统一为该模型的**每秒起步最低单价（元/秒）**：

| 官方模型标识 | 默认基准规格 | 每秒基准单价 (`ModelPrice`) | 升级高分辨率档位与倍率 | 5秒默认费用参考 | 独立图片定价项 (`-image`) | 免费图片数 |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **`MiniMax-H3`** | **768P** | **`0.50` 元/秒** | 2K (0.80元/秒) $\rightarrow$ **1.60 倍** | 2.50 元 | **`MiniMax-H3-image`**: `0.20` 元/张 | 5 张 |
| **`MiniMax-H3-Max`** | **480P** | **`0.33` 元/秒** | 768P (0.50元/秒) $\rightarrow$ **1.515152 倍** | 1.65 元 | **`MiniMax-H3-Max-image`**: `0.50` 元/张 | 2 张 |
| **`MiniMax-H3-Regeneration`** | **2K** | **`0.30` 元/秒** | 无分辨率升级档 | 1.50 元 | **`MiniMax-H3-Regeneration-image`**: `0.15` 元/张 | 5 张 |

---

## 二、 动态计费数学模型与 OtherRatios 拆解

在 New API 中，按量扣费公式为：
$$\text{总额} = \text{ModelPrice} \times \text{QuotaPerUnit} \times \text{GroupRatio} \times \prod \text{OtherRatios}$$

适配器在 `EstimateBilling` 中提供以下独立的 `OtherRatios`：

1. **时长倍率（`seconds`）**：
   $$\text{OtherRatios["seconds"]} = \text{float64}(\text{输出视频秒数})$$
   （例如：请求 5 秒就是 5.0，请求 10 秒就是 10.0，与 `sora`、`ali`、`gemini` 规范完全一致）。

2. **分辨率倍率（`resolution`）**：
   - `MiniMax-H3`：768P 为 $1.0$；2K 为 $\frac{0.80}{0.50} = \mathbf{1.60}$
   - `MiniMax-H3-Max`：**480P 为 $1.0$**；768P 为 $\frac{0.50}{0.33} \approx \mathbf{1.515152}$
   - `MiniMax-H3-Regeneration`：固定 2K 为 $1.0$

3. **附加素材倍率（`extra_assets`）**：
   若请求中包含“输入参考视频”或“超额图片”，则计算附加费用并折算为乘数：
   $$\text{基础视频费用} = \text{ModelPrice} \times \text{seconds} \times \text{ResolutionRatio}$$
   $$\text{输入视频费用} = \text{输入视频秒数} \times \text{VideoInputPricePerSec}$$
   $$\text{超额图片费用} = \max(0, \text{图片数} - \text{免费张数}) \times \text{ImagePrice}$$
   $$\text{OtherRatios["extra_assets"]} = 1.0 + \frac{\text{输入视频费用} + \text{超额图片费用}}{\text{基础视频费用}}$$

> **数学验证**：
> $\text{ModelPrice} \times \text{seconds} \times \text{ResolutionRatio} \times \text{OtherRatios["extra_assets"]} = \text{基础视频费用} + \text{输入视频费用} + \text{超额图片费用}$，分毫不差！

---

## 三、 执行任务清单 (Action Items)

### Task 1: 扩充模型与单价表定义 (`relay/channel/task/hailuo/constants.go`)
- 在 `ModelList` 中加入：`MiniMax-H3`、`MiniMax-H3-Max`、`MiniMax-H3-Regeneration`。
- 新增分辨率常量：`Resolution480P = "480P"`、`Resolution2K = "2K"`。
- 价格表采用**每秒单价**，并将 **480P 设为 H3-Max 的默认空键**：
  ```go
  type h3PriceKey struct {
      is768p bool // 针对 H3-Max 升级 768P
      is2k   bool // 针对 H3 升级 2K
  }

  var h3PriceTable = map[string]map[h3PriceKey]float64{
      "MiniMax-H3": {
          {}:           0.50, // 默认 768P: 0.50 元/秒 (基准单价)
          {is2k: true}: 0.80, // 升级 2K: 0.80 元/秒 (1.60 倍)
      },
      "MiniMax-H3-Max": {
          {}:             0.33, // 默认 480P: 0.33 元/秒 (基准单价)
          {is768p: true}: 0.50, // 升级 768P: 0.50 元/秒 (1.515152 倍)
      },
      "MiniMax-H3-Regeneration": {
          {}: 0.30, // 默认 2K: 0.30 元/秒 (基准单价)
      },
  }
  ```
- 定义各模型超额图片免费阈值、超额单价与输入视频动作单价。

### Task 2: 扩展多模态请求结构与默认规格 (`relay/channel/task/hailuo/models.go`)
- `VideoRequest` 结构体扩充多模态字段：
  - `AudioReference`、`VideoReference`、`WithAudio`。
- 在 `GetModelConfig` 中精确配置各模型默认规格：
  - `MiniMax-H3`: 默认分辨率 `Resolution768P`，默认时长 6s (官方支持 5s/6s/10s)；
  - `MiniMax-H3-Max`: **默认分辨率 `Resolution480P`**；
  - `MiniMax-H3-Regeneration`: 默认分辨率 `Resolution2K`。

### Task 3: 落实动态计费估算与素材转译 (`relay/channel/task/hailuo/adaptor.go`)
- 实现 `EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64`：
  - 提取时长，设置 `OtherRatios["seconds"] = float64(duration)`；
  - 提取分辨率，计算 `OtherRatios["resolution"]`；
  - 计算输入视频与超额图片附加费用，设置 `OtherRatios["extra_assets"]`；
  - 优先从 `ratio_setting.GetModelPrice("MiniMax-H3-image")` 动态获取管理员设置的图片单价，未设置则使用默认单价。
- 完善 `parseResolutionFromSize`（支持识别 480P/512P、2K、768P 等）。
- 完善 `convertToRequestPayload`，将 OpenAI 格式的 prompt/image/video/audio 正确转译为 MiniMax H3 请求格式。

### Task 4: 注入出厂基准单价 (`setting/ratio_setting/model_ratio.go`)
在 `defaultModelPrice` 中写入官方每秒基准单价与超额图片单价：
```go
"MiniMax-H3":                      0.50, // 768P 基准：0.50 元/秒
"MiniMax-H3-image":                0.20, // 超额图片：0.20 元/张
"MiniMax-H3-Max":                  0.33, // 480P 基准：0.33 元/秒
"MiniMax-H3-Max-image":            0.50, // Max 超额图片：0.50 元/张
"MiniMax-H3-Regeneration":         0.30, // 2K 再生成基准：0.30 元/秒
"MiniMax-H3-Regeneration-image":   0.15, // 再生成超额图片：0.15 元/张
```

### Task 5: 编写单元测试并验证 (`relay/channel/task/hailuo/adaptor_test.go`)
- 验证各模型默认规格 5 秒计费：
  - H3: 5s 768P = 2.50 元；升 2K = 4.00 元
  - H3-Max: 5s 480P = 1.65 元；升 768P = 2.50 元
  - Regen: 5s 2K = 1.50 元
- 验证 10 秒时长计费翻倍；
- 验证输入参考视频计费；
- 验证超额图片阶梯收费及管理员调价生效情况。
