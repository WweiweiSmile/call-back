package models

// 盲注默认值。用户没有设置过偏好时用它，保证录入页一打开就有值可填。
// 大盲取 1 是因为全站的筹码、金额、底池都以 BB 为单位，大盲天然就是 1bb。
const (
	DefaultSmallBlindBB = 0.5
	DefaultBigBlindBB   = 1.0
	DefaultAnteBB       = 0.0
)

// BlindConfig 盲注与前注的额度（BB）。
//
// 三者全为 0 表示"这手牌没记录盲注"，此时底池估算与加这个功能之前完全一致 ——
// 存量手牌不需要任何迁移，行为也不会变。
type BlindConfig struct {
	SmallBlindBB float64
	BigBlindBB   float64
	// AnteBB 是**每人一份**的前注，总额要乘人数
	AnteBB float64
	// TableSize 前注折算用的人数。取"几人桌"而不是实际入池人数：
	// 手牌记录里没有"谁已经离桌"这种信息，人数是唯一可依据的口径
	TableSize int
	// HeroPosition / KeyVillainPosition 用来把大小盲认到具体的人头上，见 PostedBlinds
	HeroPosition       string
	KeyVillainPosition string
}

// IsZero 这手牌没有记录任何盲注。底池文案与计算都靠它决定要不要带上盲注口径
func (b BlindConfig) IsZero() bool {
	return b.SmallBlindBB == 0 && b.BigBlindBB == 0 && b.AnteBB == 0
}

// PreflopPotBB 翻前第一条行动发生之前的底池：小盲 + 大盲 + 前注 × 人数。
//
// 前注按满员桌折算是近似值（真实牌局里空位不交前注），但比起"完全不记盲注"
// 已经准得多，而且误差方向是可控的（略偏大）。
func (b BlindConfig) PreflopPotBB() float64 {
	total := b.SmallBlindBB + b.BigBlindBB
	if b.AnteBB > 0 && b.TableSize > 0 {
		total += b.AnteBB * float64(b.TableSize)
	}
	return total
}

// PostedBlinds 大小盲分别已经算在谁头上。
//
// 这一步不能省：盲注虽然作为死钱进了底池，但下盲注的人后续跟注时只需要补差额。
// 若不把他的盲注记进他的已投入，大盲跟一个 3bb 的开池会被算成再掏 3bb（实际只需 2bb），
// 底池反而比"完全不记盲注"偏得更多。
//
// 认不出身份的盲注（例如大盲是"其他人"里的某一位）不返回：凭空挂到某个行动者名下
// 等于替一个没记录的人下注。它仍会通过 PreflopPotBB 进底池，只是不再参与差额计算。
func (b BlindConfig) PostedBlinds() map[string]float64 {
	posted := make(map[string]float64, 2)

	credit := func(actor, position string) {
		switch position {
		case PositionSB:
			posted[actor] += b.SmallBlindBB
		case PositionBB:
			posted[actor] += b.BigBlindBB
		}
	}
	credit(ActorHero, b.HeroPosition)
	credit(ActorVillain, b.KeyVillainPosition)

	return posted
}
