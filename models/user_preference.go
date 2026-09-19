package models

import "time"

// UserPreference 用户级默认设置。
//
// 两块内容：复盘录入用的盲注默认值，以及 BYOK 的模型配置。单独成表而不是往
// users 上加列：users 是登录鉴权的核心表，往上面堆业务偏好会让每次读用户信息
// 都多带一堆字段，以后新增设置项也不必再动鉴权链路。
//
// 列的所有权是硬约定：盲注三列归 PreferenceService 管，AI 四列归
// AISettingService 管，两个 service 各自只写自己的列。越界写会把对方的
// 配置清成零值——尤其别在一边的 upsert 里用字面量构造整个模型。
type UserPreference struct {
	ID     uint `json:"id" gorm:"primaryKey"`
	UserID uint `json:"userId" gorm:"not null;uniqueIndex;comment:每个用户一条"`

	SmallBlindBB float64 `json:"smallBlindBb" gorm:"default:0.5;comment:默认小盲(BB)"`
	BigBlindBB   float64 `json:"bigBlindBb" gorm:"default:1;comment:默认大盲(BB)"`
	AnteBB       float64 `json:"anteBb" gorm:"default:0;comment:默认前注(BB)，每人一份"`

	// ---------- BYOK 模型配置 ----------
	//
	// 一律用 NOT NULL DEFAULT '' 而不是可空：'' 已经统一表示"没配过"，
	// 再引入 NULL 只会多出一种需要 *string 才能表达的状态。
	//
	// AIBaseURL 为空串时的语义是"用默认预设"（config.DefaultAIPreset），
	// 不是"用户填了空"——保存时的校验会拒绝空 BaseURL，所以空串只可能
	// 出现在升级前就存在的行上
	AIBaseURL string `json:"aiBaseUrl" gorm:"size:255;not null;default:'';comment:AI 接口地址(填到 /v1 这一层)"`
	AIModel   string `json:"aiModel" gorm:"size:100;not null;default:'';comment:模型名"`

	// AIAPIKeyEncrypted 存 AES-256-GCM 密文。json:"-" 与 models.User.Password
	// 同一个理由：即使是密文也没必要下发，客户端只需要掩码
	AIAPIKeyEncrypted string `json:"-" gorm:"size:512;not null;default:'';comment:AES-256-GCM 加密后的 API Key"`

	// AIAPIKeyHint 明文存 Key 的末 4 位，只用于掩码回显。
	//
	// 刻意不靠"解密后再截取"来生成掩码：主密钥缺失或轮换后密文就解不开了，
	// 那时设置页仍要能显示"你配过一把 Key"，否则界面显示"没配过"、
	// 调用却报"配置无法解密"，两边自相矛盾
	AIAPIKeyHint string `json:"-" gorm:"size:16;not null;default:'';comment:Key 末 4 位，仅用于掩码回显"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (UserPreference) TableName() string {
	return "user_preferences"
}
