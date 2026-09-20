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
}
