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
//
// 爆炸底池是并列的另一套开局口径（见 BombPotBB）：它没有盲注，死钱来自"每人先投"。
type BlindConfig struct {
	SmallBlindBB float64
	BigBlindBB   float64
	// AnteBB 是**每人一份**的前注，总额要乘人数
	AnteBB float64
	// BombPotBB 爆炸底池每人先投的额度（BB）。> 0 表示这手是爆炸底池：
	// 没有翻前行动，所有人先投这么多直接看翻牌，起始底池 = 它 × 人数。
	//
	// 存的是**这手牌自己的额度**，而不是一个"是不是爆炸底池"的布尔标记 ——
	// 将来玩法从 5bb 改成 10bb，历史手牌不会被重新解释成 10bb。
	// 这与三个盲注列按手存而不是引用一份全局设置，是同一个理由
	BombPotBB float64
	// TableSize 前注折算用的人数。取"几人桌"而不是实际入池人数：
	// 手牌记录里没有"谁已经离桌"这种信息，人数是唯一可依据的口径。
	// 爆炸底池的每人一份也按这个口径折算
	TableSize int
	// HeroPosition / VillainPositions 用来把大小盲认到具体的人头上，见 PostedBlinds
	HeroPosition string
	// VillainPositions 记了位置的对手（M7.1 起是全部对手）。
	// 他们的行动按位置记录，所以账也记在位置这个键上
	VillainPositions []string
	// LegacyVillainPosition 老手牌里那个"关键对手"的位置，作为 ActorVillain 的兜底键。
	// M7.1 之前只有他能被认出来，且当时所有对手行动都记在聚合角色 "villain" 上，
	// 所以这笔账要记在 ActorVillain 这个键上，而不是位置键。
	// 新记录的行动按位置记，这个键不会命中，等于作废
	LegacyVillainPosition string
}

// IsZero 这手牌没有记录任何盲注。底池文案与计算都靠它决定要不要带上盲注口径。
//
// 爆炸底池下它同样是 true（那手牌确实没发盲注），但**不能据此认为底池里没有死钱** ——
// 死钱是 BombPotBB × 人数。判断玩法请用 IsBombPot
func (b BlindConfig) IsZero() bool {
	return b.SmallBlindBB == 0 && b.BigBlindBB == 0 && b.AnteBB == 0
}

// IsBombPot 这手牌是爆炸底池：没有翻前行动，每人先投 BombPotBB 直接看翻牌
func (b BlindConfig) IsBombPot() bool {
	return b.BombPotBB > 0
}

// PreflopPotBB 翻前第一条行动发生之前的底池。
//
// 常规牌局是 小盲 + 大盲 + 前注 × 人数；爆炸底池是 每人先投 × 人数（不含盲注与前注，
// 校验层保证两者互斥）。前注按满员桌折算是近似值（真实牌局里空位不交前注），
// 但比起"完全不记盲注"已经准得多，而且误差方向是可控的（略偏大）。
func (b BlindConfig) PreflopPotBB() float64 {
	if b.IsBombPot() {
		if b.TableSize <= 0 {
			return 0
		}
		// 每人一份，所以要乘人数 —— 与上面的前注同一个口径
		return b.BombPotBB * float64(b.TableSize)
	}

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
//
// 返回的键是**行动记录里的 actor 值**：我固定是 "hero"，对手是位置（M7.1 起），
// 老手牌则是聚合角色 "villain"。键对不上就等于没记账，所以两边必须一起改。
//
// 爆炸底池返回空表：没人下盲注，那笔死钱由 PreflopPotBB 按"每人先投 × 人数"一次给出。
//
// 注意爆炸底池下**每人各自的投入**没有在这里返回 —— 后端只算底池，不需要"谁还剩多少后手"，
// 所以那份计算留在前端（src/utils/poker.ts 的 bombPotContribution），别在这边补一份没人用的
func (b BlindConfig) PostedBlinds() map[string]float64 {
	posted := make(map[string]float64, 2)
	if b.IsBombPot() {
		return posted
	}

	credit := func(actor, position string) {
		switch position {
		case PositionSB:
			posted[actor] += b.SmallBlindBB
		case PositionBB:
			posted[actor] += b.BigBlindBB
		}
	}
	credit(ActorHero, b.HeroPosition)
	for _, position := range b.VillainPositions {
		credit(position, position)
	}
	credit(ActorVillain, b.LegacyVillainPosition)

	return posted
}
