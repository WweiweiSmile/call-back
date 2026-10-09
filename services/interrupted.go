package services

import (
	"call-go/config"
	"call-go/models"
	"log"
)

// ReapInterruptedTasks 把进程上次退出时留下的未完成任务判死。
//
// 后台 goroutine 随进程一起消失，但数据库里那些 pending/running 的行不会自己变 ——
// 前端会因为它一直"正在生成中"而永远转圈，追问那条更糟：它会据此禁用输入框，
// 用户既拿不到结果也没法重新提问。
//
// 放在启动时回收是**精确**判据：刚开机不可能有本进程的任务在跑。
// 多实例共享一个库时可能误伤另一个实例正在跑的任务，但那个任务完成时会把自己
// 写回终态覆盖掉，最坏只是一次瞬时错标 —— 比让用户永久卡住划算得多
//
// 只回收本文件这两张表：分析表（review_analyses）的孤儿行症状相同，但回收它还要
// 同步更新手牌上的冗余角标（见 ReviewAnalysisService.failAnalysis），
// 改动面更大，留作单独一件事
func ReapInterruptedTasks() {
	if config.DB == nil {
		return
	}

	// 退点必须排在下面那批状态更新**之前** —— 更新会把 pending/running 改成
	// failed，之后就再也找不出"哪些任务是被中断的"了
	reapOrphanRefunds()

	if res := config.DB.Model(&models.ReviewMessage{}).
		Where("role = ? AND status IN ?", models.MessageRoleAssistant,
			[]string{models.MessageStatusPending, models.MessageStatusRunning}).
		Updates(map[string]interface{}{
			"status":    models.MessageStatusFailed,
			"error_msg": chatInterruptedMsg,
		}); res.Error != nil {
		log.Printf("[回收] 追问任务失败: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("[回收] 清理了 %d 条中断的追问", res.RowsAffected)
	}

	if res := config.DB.Model(&models.ReviewProfile{}).
		Where("summary_status IN ?",
			[]string{models.SummaryStatusPending, models.SummaryStatusRunning}).
		Updates(map[string]interface{}{
			"summary_status": models.SummaryStatusFailed,
			"summary_error":  profileInterruptedMsg,
		}); res.Error != nil {
		log.Printf("[回收] 总结任务失败: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("[回收] 清理了 %d 条中断的总结重写", res.RowsAffected)
	}

	// 对手画像同理：不回收的话页面会永远停在"生成中"，
	// 而用户既拿不到结果也没法重新触发（在飞闸会把新的请求也挡掉）
	if res := config.DB.Model(&models.OpponentProfile{}).
		Where("status IN ?",
			[]string{models.SummaryStatusPending, models.SummaryStatusRunning}).
		Updates(map[string]interface{}{
			"status":    models.SummaryStatusFailed,
			"error_msg": profileInterruptedMsg,
		}); res.Error != nil {
		log.Printf("[回收] 画像任务失败: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("[回收] 清理了 %d 条中断的画像生成", res.RowsAffected)
	}
}

// reapOrphanRefunds 给被判定为"中断"的任务退点。
//
// 用户没拿到结果就不该扣（2026-10 的口径）。放在启动时回收是**精确**判据 ——
// 刚开机不可能有本进程的任务在跑。多实例共享一个库时，可能误伤另一个实例正在跑的
// 任务，从而多退几点（每条约 1~3 点）：量级可接受，比让用户白扣划算
//
// ⚠️ 刻意**不退画像总结**（review_profiles）：那张表区分不出这次重写是手动触发的
// （扣了 3 点）还是分析后自动触发的（不扣，含在复盘那 3 点里）。无差别退会给
// 自动那条白送点数。要修得先在那张表上记一个"这次扣没扣"的标记
//
// 只统计、不改状态：调用方紧接着就会把那批行标成 failed
func reapOrphanRefunds() {
	quota := &AIQuotaService{}

	type userCount struct {
		UserID uint
		N      int64
	}

	// 追问：每条 pending/running 的 assistant 占位行，都是在 ReviewChatService.Ask
	// 里扣过 1 点的 —— 占位行与扣点同生共死
	var chats []userCount
	if err := config.DB.Model(&models.ReviewMessage{}).
		Select("user_id, COUNT(*) AS n").
		Where("role = ? AND status IN ?", models.MessageRoleAssistant,
			[]string{models.MessageStatusPending, models.MessageStatusRunning}).
		Group("user_id").Scan(&chats).Error; err != nil {
		log.Printf("[回收] 统计待退点的追问失败: %v", err)
	} else {
		for _, c := range chats {
			quota.RefundPoints(c.UserID, int(c.N)*config.AICostChat)
		}
	}

	// 对手画像：每次生成（含首次）都扣了 3 点
	var profiles []userCount
	if err := config.DB.Model(&models.OpponentProfile{}).
		Select("user_id, COUNT(*) AS n").
		Where("status IN ?",
			[]string{models.SummaryStatusPending, models.SummaryStatusRunning}).
		Group("user_id").Scan(&profiles).Error; err != nil {
		log.Printf("[回收] 统计待退点的画像失败: %v", err)
	} else {
		for _, p := range profiles {
			quota.RefundPoints(p.UserID, int(p.N)*config.AICostOpponentProfile)
		}
	}
}
