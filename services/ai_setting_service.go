package services

import (
	"call-go/config"
	"call-go/dto"
	"call-go/models"
	"call-go/utils"
	"errors"
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"
)

// aiNotConfiguredMsg 所有"没配模型"场景共用的一句提示。
//
// 三个调用链路（分析 / 追问 / 画像总结）都会把它透给用户，文案必须一致且可行动：
// 只说要做什么，不说"未启用"这种用户无从下手的描述
const aiNotConfiguredMsg = "还没配置 AI 模型，请到「设置 → 模型设置」填写你自己的 API Key"

// AISettingService 用户的模型配置（BYOK：地址 + 模型名 + 用户自己的 API Key）。
//
// ⚠️ 必须是叶子服务：只依赖 config / models / utils。
// ReviewAnalysisService 与 ReviewMemoryService 都会用它，一旦它反过来依赖
// 它们中的任何一个就会成环
type AISettingService struct{}

// Get 读我的模型配置。
//
// 没配过的用户拿到默认预设，不是 404，且**不写库** —— 与 PreferenceService.Get
// 同一条原则：库里分不出"他选的正好是默认值"和"他从没设置过"
func (s *AISettingService) Get(userID uint) (*dto.AIModelSettingsResponse, error) {
	pref, err := s.loadPreferenceOrNew(userID)
	if err != nil {
		return nil, err
	}

	baseURL, model := resolveAIValues(pref.AIBaseURL, pref.AIModel)
	presets := dto.ToAIModelPresetResponses(config.AIPresets())

	// 只传密文的存在性与尾号，明文 Key 不进这个函数
	return dto.ToAIModelSettingsResponse(
		baseURL, model, pref.AIAPIKeyHint, pref.AIAPIKeyEncrypted != "", presets,
	), nil
}

// Update 保存模型配置。
//
// 只写 AI 四列。盲注三列在 loadPreferenceOrNew 里要么带着库里的原值、
// 要么是新建时的默认值，两种情况下都不会被这一路请求改坏
func (s *AISettingService) Update(userID uint, req *dto.UpdateAIModelSettingsRequest) (*dto.AIModelSettingsResponse, error) {
	baseURL, err := utils.ValidateAIBaseURL(req.BaseURL)
	if err != nil {
		return nil, err
	}
	if err := utils.ValidateAIModel(req.Model); err != nil {
		return nil, err
	}

	pref, err := s.loadPreferenceOrNew(userID)
	if err != nil {
		return nil, err
	}

	// 三态决定 Key 的走向。加密失败（主密钥缺失）必须原样上报，
	// 绝不能降级成"先把 Key 清掉"——那会连带毁掉用户原有的配置
	cipher, hint, err := decideAPIKey(pref.AIAPIKeyEncrypted, pref.AIAPIKeyHint, req.APIKey)
	if err != nil {
		return nil, err
	}

	pref.AIBaseURL = baseURL
	pref.AIModel = strings.TrimSpace(req.Model)
	pref.AIAPIKeyEncrypted = cipher
	pref.AIAPIKeyHint = hint

	if pref.ID == 0 {
		if err := config.DB.Create(pref).Error; err != nil {
			return nil, err
		}
	} else if err := config.DB.Save(pref).Error; err != nil {
		return nil, err
	}

	return dto.ToAIModelSettingsResponse(baseURL, pref.AIModel, hint, cipher != "", nil), nil
}

// ResolveCallSettings 解析该用户本次模型调用要用的凭据。
//
// 错误文案会经 error_msg 落库并展示给用户，所以每一句都必须可行动 ——
// 用户看完就知道要去哪儿改什么
func (s *AISettingService) ResolveCallSettings(userID uint) (*AICallSettings, error) {
	pref, err := s.loadPreferenceOrNew(userID)
	if err != nil {
		return nil, err
	}

	if pref.AIAPIKeyEncrypted == "" {
		return nil, errors.New(aiNotConfiguredMsg)
	}

	apiKey, err := utils.DecryptSecret(pref.AIAPIKeyEncrypted)
	if err != nil {
		// 主密钥缺失或轮换过都会走到这里。用户那边看到的应该是"重新填一把"，
		// 而具体原因留在服务端日志里排查
		log.Printf("[模型配置] 解密失败 user=%d: %v", userID, err)
		return nil, errors.New("模型配置无法解密，请到「设置 → 模型设置」重新填写 API Key")
	}

	baseURL, model := resolveAIValues(pref.AIBaseURL, pref.AIModel)

	// 保存时已经校验过，这里再跑一次是第二道闸：防的是有人直接改库
	// 塞进一个内网地址。纯函数，成本为零
	baseURL, err = utils.ValidateAIBaseURL(baseURL)
	if err != nil {
		log.Printf("[模型配置] 地址不合法 user=%d: %v", userID, err)
		return nil, fmt.Errorf("模型配置里的地址不可用（%s），请到「设置 → 模型设置」修改", err.Error())
	}
	if err := utils.ValidateAIModel(model); err != nil {
		return nil, fmt.Errorf("模型配置不完整（%s），请到「设置 → 模型设置」补全", err.Error())
	}

	return &AICallSettings{APIKey: apiKey, BaseURL: baseURL, Model: model}, nil
}

// HasUsableConfig 该用户是否配了**可用**的模型，供前端决定按钮是否可点。
//
// 刻意不做"有密文就算 true"：主密钥轮换后密文还在，但调用必然失败。
// 那时前端显示"开始分析"再必然失败，比直接显示"还没配置"更糟
func (s *AISettingService) HasUsableConfig(userID uint) bool {
	_, err := s.ResolveCallSettings(userID)
	return err == nil
}

// loadPreferenceOrNew 取该用户的行；没有行时返回一条带盲注默认值的**未落库**记录。
//
// 注意它不写库：Get 与 ResolveCallSettings 只用返回值读 AI 字段，
// 只有 Update 才会把结果落盘
func (s *AISettingService) loadPreferenceOrNew(userID uint) (*models.UserPreference, error) {
	var pref models.UserPreference
	err := config.DB.Where("user_id = ?", userID).First(&pref).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 建行时必须带上盲注默认值，理由见 newPreferenceDefaults
		return newPreferenceDefaults(userID), nil
	}
	if err != nil {
		return nil, err
	}
	return &pref, nil
}

// resolveAIValues 存储值为空串时回退到默认预设。
//
// 空串只可能出现在升级前就存在的行上：保存时的校验会拒绝空 BaseURL 与空模型名。
// 有了这层回退，老用户升级后一打开设置页看到的预填值就是好的，只需粘一把 Key
func resolveAIValues(storedBaseURL, storedModel string) (string, string) {
	preset := config.DefaultAIPreset()

	baseURL := strings.TrimSpace(storedBaseURL)
	if baseURL == "" {
		baseURL = preset.BaseURL
	}
	model := strings.TrimSpace(storedModel)
	if model == "" {
		model = preset.Model
	}
	return baseURL, model
}

// decideAPIKey 按三态决定 Key 的走向，返回（密文, 尾号）。
//
//	incoming == nil   → 不动，原样返回。**「只改模型名」靠的就是这一条**
//	*incoming == ""   → 清除
//	其它              → 校验后加密替换
func decideAPIKey(storedCipher, storedHint string, incoming *string) (string, string, error) {
	if incoming == nil {
		return storedCipher, storedHint, nil
	}

	trimmed := strings.TrimSpace(*incoming)
	if trimmed == "" {
		return "", "", nil
	}

	if err := utils.ValidateAPIKey(trimmed); err != nil {
		return "", "", err
	}

	cipher, err := utils.EncryptSecret(trimmed)
	if err != nil {
		return "", "", err
	}
	return cipher, utils.SecretHint(trimmed), nil
}
