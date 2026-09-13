package services

import (
	"call-go/config"
	"call-go/dto"
	"call-go/models"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ErrRequestAlreadyHandled 申请已被处理（重复审批）
var ErrRequestAlreadyHandled = errors.New("该申请已被处理")

type ScoreRequestService struct{}

// ScoreRequestQuery 申请列表查询条件
type ScoreRequestQuery struct {
	GameID   uint
	Status   string
	Scope    string // mine-我提交的, review-我创建的场次待审
	Page     int
	PageSize int
}

// Create 提交存取分申请（申请人一律取自 JWT）
func (s *ScoreRequestService) Create(userID uint, req *dto.CreateScoreRequestRequest) (*dto.ScoreRequestResponse, error) {
	var game models.Game
	if err := config.DB.First(&game, req.GameID).Error; err != nil {
		return nil, errors.New("场次不存在")
	}
	if game.IsEnded() {
		return nil, errors.New("场次已结束")
	}
	// 创建者走直接的存取分接口，不需要申请
	if game.CreatorID == userID {
		return nil, errors.New("场次创建者无需申请，可直接操作")
	}
	if err := ensureActiveParticipant(config.DB, req.GameID, userID); err != nil {
		return nil, err
	}

	// 注意：取分不校验余额是否充足——本业务的余额允许为负，
	// 参与者可以取走超过已存入的分数（这正是"不平衡"的来源）。

	request := &models.ScoreRequest{
		GameID: req.GameID,
		UserID: userID,
		Type:   req.Type,
		Amount: req.Amount,
		Remark: req.Remark,
		Status: models.ScoreReqStatusPending,
	}

	applicantName := displayNameOf(config.DB, userID)

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(request).Error; err != nil {
			return err
		}
		// 通知场次创建者有待审申请
		return pushMessage(tx, game.CreatorID, models.MsgTypeRequestCreated,
			"新的存取分申请",
			fmt.Sprintf("%s 提交了%s申请 %s 分", applicantName, typeLabel(req.Type), formatAmount(req.Amount)),
			&game.ID, &request.ID)
	})
	if err != nil {
		return nil, err
	}

	responses := s.fillResponses([]models.ScoreRequest{*request})
	return &responses[0], nil
}

// GetList 申请列表。scope=mine 只能看自己的；scope=review 只能是场次创建者
func (s *ScoreRequestService) GetList(operatorID uint, q ScoreRequestQuery) (*dto.ScoreRequestListResponse, error) {
	query := config.DB.Model(&models.ScoreRequest{})

	switch q.Scope {
	case "review":
		if q.GameID == 0 {
			return nil, errors.New("查看待审列表需要指定场次")
		}
		var game models.Game
		if err := config.DB.First(&game, q.GameID).Error; err != nil {
			return nil, errors.New("场次不存在")
		}
		if game.CreatorID != operatorID {
			return nil, errors.New("无权查看该场次的申请")
		}
		query = query.Where("game_id = ?", q.GameID)
	default: // mine
		query = query.Where("user_id = ?", operatorID)
		if q.GameID > 0 {
			query = query.Where("game_id = ?", q.GameID)
		}
	}

	if q.Status != "" {
		query = query.Where("status = ?", q.Status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var requests []models.ScoreRequest
	offset := (q.Page - 1) * q.PageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(q.PageSize).Find(&requests).Error; err != nil {
		return nil, err
	}

	return &dto.ScoreRequestListResponse{
		Total: total,
		List:  s.fillResponses(requests),
	}, nil
}

// Approve 审核通过：分数入库 + 通知申请人。
//
// 幂等与并发：事务内第一条语句就是带 status='pending' 的条件 UPDATE 抢单，
// 并发下只有一个请求能命中（RowsAffected==1），另一个拿到 0 行直接报错。
// 权限与状态校验放在抢单之后——任何一步失败都会回滚整个事务，抢单一起撤销，
// 申请保持 pending 可重试。
func (s *ScoreRequestService) Approve(reviewerID, requestID uint, req *dto.ReviewScoreRequestRequest) (*dto.ScoreRequestResponse, error) {
	var result models.ScoreRequest

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.ScoreRequest{}).
			Where("id = ? AND status = ?", requestID, models.ScoreReqStatusPending).
			Updates(map[string]any{
				"status":        models.ScoreReqStatusApproved,
				"reviewer_id":   reviewerID,
				"reviewed_at":   time.Now(),
				"review_remark": req.ReviewRemark,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrRequestAlreadyHandled
		}

		// 该行已被本事务持锁，读到的必然是自己抢到的状态
		if err := tx.First(&result, requestID).Error; err != nil {
			return err
		}

		// 重新校验：提交到审批之间场次可能已结束、申请人可能已退出
		var game models.Game
		if err := tx.First(&game, result.GameID).Error; err != nil {
			return errors.New("场次不存在")
		}
		if game.CreatorID != reviewerID {
			return errors.New("无权审核该申请")
		}
		if game.IsEnded() {
			return errors.New("场次已结束")
		}
		if err := ensureActiveParticipant(tx, result.GameID, result.UserID); err != nil {
			return err
		}

		// 入库。操作人记为审批人、类型 proxy：谁点的通过要能在流水里查到，
		// 同时保持 self/proxy 与 user_id==operator_id 的不变量成立。
		// 取分不校验余额，允许取成负数。
		trans, err := applyBalanceChange(tx, balanceOpParams{
			UserID:       result.UserID,
			GameID:       result.GameID,
			TransType:    result.Type,
			Amount:       result.Amount,
			OperatorID:   reviewerID,
			OperatorType: models.OperatorTypeProxy,
			Remark:       approvedRemark(&result),
			AllowCreate:  result.Type == models.ScoreReqTypeDeposit,
		})
		if err != nil {
			return err
		}

		if err := tx.Model(&models.ScoreRequest{}).Where("id = ?", result.ID).
			Update("transaction_id", trans.ID).Error; err != nil {
			return err
		}
		result.TransactionID = &trans.ID

		return pushMessage(tx, result.UserID, models.MsgTypeRequestApproved,
			"存取分申请已通过",
			fmt.Sprintf("你提交的%s申请 %s 分已通过审核", typeLabel(result.Type), formatAmount(result.Amount)),
			&result.GameID, &result.ID)
	})
	if err != nil {
		return nil, err
	}

	responses := s.fillResponses([]models.ScoreRequest{result})
	return &responses[0], nil
}

// Reject 驳回申请：不动余额，只改状态并通知申请人
func (s *ScoreRequestService) Reject(reviewerID, requestID uint, req *dto.ReviewScoreRequestRequest) (*dto.ScoreRequestResponse, error) {
	reason := strings.TrimSpace(req.ReviewRemark)
	if reason == "" {
		return nil, errors.New("请填写驳回理由")
	}

	var result models.ScoreRequest

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.ScoreRequest{}).
			Where("id = ? AND status = ?", requestID, models.ScoreReqStatusPending).
			Updates(map[string]any{
				"status":        models.ScoreReqStatusRejected,
				"reviewer_id":   reviewerID,
				"reviewed_at":   time.Now(),
				"review_remark": reason,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrRequestAlreadyHandled
		}

		if err := tx.First(&result, requestID).Error; err != nil {
			return err
		}

		var game models.Game
		if err := tx.First(&game, result.GameID).Error; err != nil {
			return errors.New("场次不存在")
		}
		if game.CreatorID != reviewerID {
			return errors.New("无权审核该申请")
		}

		return pushMessage(tx, result.UserID, models.MsgTypeRequestRejected,
			"存取分申请被驳回",
			fmt.Sprintf("你提交的%s申请 %s 分被驳回：%s", typeLabel(result.Type), formatAmount(result.Amount), reason),
			&result.GameID, &result.ID)
	})
	if err != nil {
		return nil, err
	}

	responses := s.fillResponses([]models.ScoreRequest{result})
	return &responses[0], nil
}

// Cancel 申请人撤销自己的待审申请
func (s *ScoreRequestService) Cancel(userID, requestID uint) error {
	res := config.DB.Model(&models.ScoreRequest{}).
		Where("id = ? AND user_id = ? AND status = ?", requestID, userID, models.ScoreReqStatusPending).
		Update("status", models.ScoreReqStatusCancelled)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("申请不存在或已被处理")
	}
	return nil
}

// fillResponses 批量填充申请人、审核人与场次名称，避免逐条查库
func (s *ScoreRequestService) fillResponses(requests []models.ScoreRequest) []dto.ScoreRequestResponse {
	userIDs := make([]uint, 0, len(requests)*2)
	gameIDs := make([]uint, 0, len(requests))
	for _, r := range requests {
		userIDs = append(userIDs, r.UserID)
		if r.ReviewerID != nil {
			userIDs = append(userIDs, *r.ReviewerID)
		}
		gameIDs = append(gameIDs, r.GameID)
	}

	userMap := batchUserNames(userIDs)
	gameMap := batchGameNames(gameIDs)

	list := make([]dto.ScoreRequestResponse, 0, len(requests))
	for _, r := range requests {
		item := dto.ScoreRequestResponse{
			ID:            r.ID,
			GameID:        r.GameID,
			GameName:      gameMap[r.GameID],
			UserID:        r.UserID,
			UserName:      userMap[r.UserID],
			Type:          r.Type,
			Amount:        r.Amount,
			Remark:        r.Remark,
			Status:        r.Status,
			ReviewerID:    r.ReviewerID,
			ReviewRemark:  r.ReviewRemark,
			ReviewedAt:    r.ReviewedAt,
			TransactionID: r.TransactionID,
			CreatedAt:     r.CreatedAt,
		}
		if r.ReviewerID != nil {
			item.ReviewerName = userMap[*r.ReviewerID]
		}
		list = append(list, item)
	}
	return list
}

// batchUserNames 批量取用户显示名（昵称优先，回退用户名）
func batchUserNames(ids []uint) map[uint]string {
	result := make(map[uint]string)
	if len(ids) == 0 {
		return result
	}

	unique := make([]uint, 0, len(ids))
	seen := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	var users []models.User
	config.DB.Where("id IN ?", unique).Find(&users)
	for _, u := range users {
		if u.Nickname != "" {
			result[u.ID] = u.Nickname
		} else {
			result[u.ID] = u.Username
		}
	}
	return result
}

// batchGameNames 批量取场次名称
func batchGameNames(ids []uint) map[uint]string {
	result := make(map[uint]string)
	if len(ids) == 0 {
		return result
	}

	var games []models.Game
	config.DB.Where("id IN ?", ids).Find(&games)
	for _, g := range games {
		result[g.ID] = g.Name
	}
	return result
}

// displayNameOf 取单个用户的显示名
func displayNameOf(db *gorm.DB, userID uint) string {
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		return ""
	}
	if user.Nickname != "" {
		return user.Nickname
	}
	return user.Username
}

// typeLabel 申请类型的中文名
func typeLabel(transType string) string {
	if transType == models.ScoreReqTypeWithdraw {
		return "取分"
	}
	return "存分"
}

// formatAmount 金额千分位
func formatAmount(amount int64) string {
	text := fmt.Sprintf("%d", amount)
	if amount < 0 {
		return text
	}

	var parts []string
	for len(text) > 3 {
		parts = append([]string{text[len(text)-3:]}, parts...)
		text = text[:len(text)-3]
	}
	parts = append([]string{text}, parts...)
	return strings.Join(parts, ",")
}

// approvedRemark 审批通过后写入交易流水的备注，保留申请出处
func approvedRemark(request *models.ScoreRequest) string {
	if strings.TrimSpace(request.Remark) == "" {
		return "申请" + typeLabel(request.Type) + "（无备注）"
	}
	return "申请" + typeLabel(request.Type) + "：" + request.Remark
}
