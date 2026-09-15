package utils

import "call-go/models"

// PotStep 一条街的底池推进
type PotStep struct {
	Street     string
	PotStartBB float64
	PotEndBB   float64
}

// ComputeStreetPots 按记录的行动估算每条街的底池。
//
// 跟注金额不要求用户填 —— 它一定等于当前街的最高下注额，能推出来。
// 加注记录的是"加到多少"，所以本街净投入要用当前投入去减。
//
// 不计入盲注与前注：盲注级别是自由文本，且存在抓头/前注等变体，
// 硬猜会让数字看起来精确但其实是错的。调用方必须在展示时标明这是估算值。
//
// 注意 前端 src/utils/poker.ts 里有一份等价实现，那份只用于录入时的实时显示，
// 不会回写数据库（StreetRecord.PotStartBB 保持为空）。这里是给模型看的版本，
// 是唯一的权威计算。两份逻辑若有一处调整，另一处也要跟着改。
func ComputeStreetPots(streets []models.StreetRecord) map[string]PotStep {
	result := make(map[string]PotStep, len(streets))
	pot := 0.0

	for _, street := range []string{models.StreetPreflop, models.StreetFlop, models.StreetTurn, models.StreetRiver} {
		potStart := pot

		var record *models.StreetRecord
		for i := range streets {
			if streets[i].Street == street {
				record = &streets[i]
				break
			}
		}

		if record != nil && len(record.Actions) > 0 {
			// 本街每个行动者的已投入。other 是聚合角色，多个"其他人"共用一个桶，
			// 属于简化处理，但不会影响底池总额
			contributed := make(map[string]float64, 3)
			currentBet := 0.0

			for _, action := range record.Actions {
				prev := contributed[action.Actor]
				amount := 0.0
				if action.AmountBB != nil {
					amount = *action.AmountBB
				}

				delta := 0.0
				switch action.Action {
				case models.ActionBet, models.ActionRaise, models.ActionAllin:
					// 这三种记录的都是"总投入额"，所以要减掉本街已投入的部分
					if amount > prev {
						delta = amount - prev
					}
					if amount > currentBet {
						currentBet = amount
					}
				case models.ActionCall:
					// 跟注补齐到当前街的最高下注额
					if currentBet > prev {
						delta = currentBet - prev
					}
				default:
					// check / fold 不投入
				}

				contributed[action.Actor] = prev + delta
				pot += delta
			}
		}

		result[street] = PotStep{Street: street, PotStartBB: potStart, PotEndBB: pot}
	}

	return result
}
