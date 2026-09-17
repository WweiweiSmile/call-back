package services

import (
	"call-go/config"
	"call-go/dto"
	"call-go/models"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ErrSuggestionAlreadyHandled 建议已被处理（重复审批）
var ErrSuggestionAlreadyHandled = errors.New("该标签建议已被处理")

// 单条标签名的长度上限。与 review_leak_tags.name 的列宽一致——
// 超了会被 MySQL 静默截断，两条不同的建议就撞成同一个去重键了
const suggestionNameMaxRunes = 64

// suggestionReasonMaxRunes 消息正文里展示的理由长度。完整理由留在库里，
// 详情页能看到，消息中心只是提醒
const suggestionReasonMaxRunes = 80

type TagSuggestionService struct{}

// RecordSuggestions 把一次分析产出的新标签建议并入待审队列。
//
// 全程只记日志不返回错误：分析结果本身已经落库，入队是增量收益，
// 不该因为它出问题就把这手牌标成失败、让用户白花一次额度
// （与 recordMemory 同一个取舍）。
func (s *TagSuggestionService) RecordSuggestions(analysisID uint, items []models.SuggestedTagItem) {
	for _, item := range items {
		name := truncateRunes(strings.TrimSpace(item.Name), suggestionNameMaxRunes)
		if name == "" {
			continue
		}

		suggestion, created, err := upsertSuggestion(analysisID, name, strings.TrimSpace(item.Reason))
		if err != nil {
			log.Printf("[标签建议] 入队失败，name=%q: %v", name, err)
			continue
		}
		// 只在第一次见到这个标签时通知，否则同一名字每被提议一次就骚扰一遍管理员
		if created {
			notifyAdmins(suggestion.ID, name, item.Reason)
		}
	}
}

// upsertSuggestion 按 name 合并建议，返回合并后的记录与"是不是这次新建的"
func upsertSuggestion(analysisID uint, name, reason string) (*models.ReviewTagSuggestion, bool, error) {
	var existing models.ReviewTagSuggestion
	err := config.DB.Where("name = ?", name).First(&existing).Error
	if err == nil {
		// 已存在：累加计数。刻意不动 status——被拒绝过的名字再次出现时
		// 不该自动回到待审，否则管理员每拒绝一次都要再拒一次
		err = config.DB.Model(&models.ReviewTagSuggestion{}).
			Where("id = ?", existing.ID).
			Updates(map[string]any{
				"hit_count":        gorm.Expr("hit_count + 1"),
				"last_analysis_id": analysisID,
			}).Error
		return &existing, false, err
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	suggestion := &models.ReviewTagSuggestion{
		Name:            name,
		Reason:          truncateRunes(reason, 500),
		Status:          models.TagSuggestionStatusPending,
		HitCount:        1,
		FirstAnalysisID: analysisID,
		LastAnalysisID:  analysisID,
	}
	if err := config.DB.Create(suggestion).Error; err != nil {
		// 并发下同名的两次分析可能同时走到这里，后到的会撞唯一索引。
		// 这不是错误，退回累加即可
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			if err := config.DB.Where("name = ?", name).First(&existing).Error; err != nil {
				return nil, false, err
			}
			err = config.DB.Model(&models.ReviewTagSuggestion{}).
				Where("id = ?", existing.ID).
				Updates(map[string]any{
					"hit_count":        gorm.Expr("hit_count + 1"),
					"last_analysis_id": analysisID,
				}).Error
			return &existing, false, err
		}
		return nil, false, err
	}
	return suggestion, true, nil
}

// notifyAdmins 给所有系统管理发待审通知。
// 标签字典是所有人共用的，所以任何人的分析产出建议都要让管理员看到
func notifyAdmins(suggestionID uint, name, reason string) {
	var admins []models.User
	if err := config.DB.
		Where("role = ? AND status = ?", models.UserRoleAdmin, "active").
		Find(&admins).Error; err != nil {
		log.Printf("[标签建议] 查询管理员失败: %v", err)
		return
	}

	// 一个管理员都没有时静默跳过：建议仍留在队列里，
	// 之后指定了管理员也不会丢，只是当时没人收到通知
	title := "新的漏洞标签待审"
	content := fmt.Sprintf("AI 提议新标签「%s」，理由：%s",
		name, shortenMsg(reason, suggestionReasonMaxRunes))

	for _, admin := range admins {
		if err := pushMessage(config.DB, admin.ID, models.MsgTypeTagSuggestionPending,
			title, content, messageRef{SuggestionID: &suggestionID}); err != nil {
			log.Printf("[标签建议] 通知管理员 %d 失败: %v", admin.ID, err)
		}
	}
}

// Approve 审批通过：补齐词典字段后写入 review_leak_tags，对所有人立即生效
func (s *TagSuggestionService) Approve(operatorID, suggestionID uint, req *dto.ApproveTagSuggestionRequest) (*dto.TagSuggestionResponse, error) {
	if err := ensureAdmin(operatorID); err != nil {
		return nil, err
	}

	code := strings.TrimSpace(req.Code)
	name := strings.TrimSpace(req.Name)
	if code == "" || name == "" {
		return nil, errors.New("标签 code 与名称不能为空")
	}

	var result models.ReviewTagSuggestion

	// 抢单模式，与 ScoreRequestService.Approve 同款：第一条语句是带
	// status='pending' 的条件 UPDATE，并发下只有一个管理员能批中。
	// 后续任何一步失败都会回滚整个事务，建议保持待审可重试
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.ReviewTagSuggestion{}).
			Where("id = ? AND status = ?", suggestionID, models.TagSuggestionStatusPending).
			Updates(map[string]any{
				"status":        models.TagSuggestionStatusApproved,
				"reviewer_id":   operatorID,
				"reviewed_at":   time.Now(),
				"review_remark": "",
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrSuggestionAlreadyHandled
		}

		if err := tx.First(&result, suggestionID).Error; err != nil {
			return err
		}

		// code 是模型引用标签的唯一标识，重复会让提示词里出现两个同 code 的标签，
		// 模型选哪个都说得通——必须挡住，让管理员换一个再提交
		var dup int64
		if err := tx.Model(&models.ReviewLeakTag{}).
			Where("code = ?", code).Count(&dup).Error; err != nil {
			return err
		}
		if dup > 0 {
			return fmt.Errorf("标签 code「%s」已存在，请换一个", code)
		}

		tag := models.ReviewLeakTag{
			Code:        code,
			Name:        truncateRunes(name, 64),
			Category:    req.Category,
			Description: truncateRunes(strings.TrimSpace(req.Description), 255),
			// 入库即对所有人生效，这正是"标签库所有人通用"的含义
			IsActive:  true,
			SortOrder: nextTagSortOrder(tx, req.Category),
		}
		if err := tx.Create(&tag).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.ReviewTagSuggestion{}).
			Where("id = ?", result.ID).
			Update("tag_id", tag.ID).Error; err != nil {
			return err
		}
		result.TagID = &tag.ID

		return nil
	})
	if err != nil {
		return nil, err
	}

	return toTagSuggestionResponse(&result), nil
}

// Reject 驳回：建议不进词典，且同名建议之后不会再回到待审
func (s *TagSuggestionService) Reject(operatorID, suggestionID uint, req *dto.RejectTagSuggestionRequest) (*dto.TagSuggestionResponse, error) {
	if err := ensureAdmin(operatorID); err != nil {
		return nil, err
	}

	reason := strings.TrimSpace(req.ReviewRemark)
	if reason == "" {
		return nil, errors.New("请填写驳回理由")
	}

	res := config.DB.Model(&models.ReviewTagSuggestion{}).
		Where("id = ? AND status = ?", suggestionID, models.TagSuggestionStatusPending).
		Updates(map[string]any{
			"status":        models.TagSuggestionStatusRejected,
			"reviewer_id":   operatorID,
			"reviewed_at":   time.Now(),
			"review_remark": reason,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrSuggestionAlreadyHandled
	}

	var result models.ReviewTagSuggestion
	if err := config.DB.First(&result, suggestionID).Error; err != nil {
		return nil, err
	}
	return toTagSuggestionResponse(&result), nil
}

// ensureAdmin 校验操作人是系统管理。
// 权限只在 service 层判一次，与既有的 game.CreatorID 判断同款，不引入中间件
func ensureAdmin(userID uint) error {
	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		return errors.New("用户不存在")
	}
	if !user.IsAdmin() {
		return errors.New("只有系统管理可以审批标签")
	}
	return nil
}

// nextTagSortOrder 新标签排在同分类末尾。
// 直接给 0 的话，AI 入库的标签会排到字典最前面，人工整理好的顺序就乱了
func nextTagSortOrder(tx *gorm.DB, category string) int {
	var max int
	tx.Model(&models.ReviewLeakTag{}).
		Where("category = ?", category).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&max)
	return max + 10
}

func toTagSuggestionResponse(s *models.ReviewTagSuggestion) *dto.TagSuggestionResponse {
	return &dto.TagSuggestionResponse{
		ID:           s.ID,
		Name:         s.Name,
		Reason:       s.Reason,
		Status:       s.Status,
		HitCount:     s.HitCount,
		ReviewerID:   s.ReviewerID,
		ReviewRemark: s.ReviewRemark,
		ReviewedAt:   s.ReviewedAt,
		TagID:        s.TagID,
		CreatedAt:    s.CreatedAt,
	}
}
