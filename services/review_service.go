package services

import (
	"call-go/config"
	"call-go/dto"
	"call-go/models"
	"call-go/utils"
	"errors"
	"strings"

	"gorm.io/gorm"
)

// ReviewHandFilter 手牌列表筛选条件
type ReviewHandFilter struct {
	Position      string
	Tag           string
	GameID        *uint
	AnalyzeStatus string
	Keyword       string
	Page          int
	PageSize      int
}

type ReviewService struct{}

// CreateHand 创建复盘手牌
func (s *ReviewService) CreateHand(userID uint, req *dto.ReviewHandRequest) (*models.ReviewHand, error) {
	hand := &models.ReviewHand{
		UserID:        userID,
		AnalyzeStatus: models.AnalyzeStatusNone,
	}
	applyHandRequest(hand, req)

	if err := utils.ValidateReviewHand(hand); err != nil {
		return nil, err
	}

	if err := s.checkGameAccessible(userID, hand.GameID); err != nil {
		return nil, err
	}

	if hand.Title == "" {
		hand.Title = buildHandTitle(hand)
	}
	hand.ContentHash = utils.ComputeHandHash(hand)

	if err := config.DB.Create(hand).Error; err != nil {
		return nil, err
	}
	return hand, nil
}

// GetHandList 获取当前用户的复盘手牌列表
func (s *ReviewService) GetHandList(userID uint, f ReviewHandFilter) (*dto.ReviewHandListResponse, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 10
	}

	query := config.DB.Model(&models.ReviewHand{}).Where("user_id = ?", userID)

	if f.Position != "" {
		query = query.Where("hero_position = ?", strings.ToUpper(f.Position))
	}
	if f.GameID != nil {
		query = query.Where("game_id = ?", *f.GameID)
	}
	if f.AnalyzeStatus != "" {
		query = query.Where("analyze_status = ?", f.AnalyzeStatus)
	}
	if f.Tag != "" {
		// hero_tags 是 JSON 列，用 JSON_CONTAINS 判断数组成员。
		// JSON_QUOTE 把参数包成 JSON 字符串字面量，不能用普通 = 比较
		query = query.Where("JSON_CONTAINS(hero_tags, JSON_QUOTE(?))", f.Tag)
	}
	if f.Keyword != "" {
		// 转义 LIKE 的通配符，避免用户输入的 % 变成通配导致全表扫描
		kw := "%" + escapeLike(f.Keyword) + "%"
		query = query.Where("(title LIKE ? OR hero_thought LIKE ?)", kw, kw)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var hands []models.ReviewHand
	offset := (f.Page - 1) * f.PageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(f.PageSize).Find(&hands).Error; err != nil {
		return nil, err
	}

	gameNames := s.loadGameNames(hands)

	list := make([]dto.ReviewHandResponse, 0, len(hands))
	for i := range hands {
		var gameName string
		if hands[i].GameID != nil {
			gameName = gameNames[*hands[i].GameID]
		}
		list = append(list, dto.ToReviewHandResponse(&hands[i], gameName))
	}

	return &dto.ReviewHandListResponse{Total: total, List: list}, nil
}

// GetHand 获取手牌详情。
// 查询条件带上 user_id：别人的手牌对当前用户表现为"不存在"，不泄露存在性。
func (s *ReviewService) GetHand(userID, handID uint) (*models.ReviewHand, error) {
	var hand models.ReviewHand
	err := config.DB.Where("id = ? AND user_id = ?", handID, userID).First(&hand).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("手牌不存在")
		}
		return nil, err
	}
	return &hand, nil
}

// GetHandDetail 获取手牌详情响应（已填充关联场次名）
func (s *ReviewService) GetHandDetail(userID, handID uint) (*dto.ReviewHandResponse, error) {
	hand, err := s.GetHand(userID, handID)
	if err != nil {
		return nil, err
	}

	names := s.loadGameNames([]models.ReviewHand{*hand})
	var gameName string
	if hand.GameID != nil {
		gameName = names[*hand.GameID]
	}

	resp := dto.ToReviewHandResponse(hand, gameName)
	return &resp, nil
}

// UpdateHand 整体替换更新手牌
func (s *ReviewService) UpdateHand(userID, handID uint, req *dto.ReviewHandRequest) (*models.ReviewHand, error) {
	hand, err := s.GetHand(userID, handID)
	if err != nil {
		return nil, err
	}

	oldHash := hand.ContentHash
	applyHandRequest(hand, req)

	if err := utils.ValidateReviewHand(hand); err != nil {
		return nil, err
	}
	if err := s.checkGameAccessible(userID, hand.GameID); err != nil {
		return nil, err
	}

	if hand.Title == "" {
		hand.Title = buildHandTitle(hand)
	}
	hand.ContentHash = utils.ComputeHandHash(hand)

	// 内容变了，之前的分析结论就不再对应当前这手牌。
	// 置回 none 表示"当前内容尚未分析"，M3 会据此判断是否需要重新调用模型；
	// 历史分析记录会保留在 analysis 表里，不会丢。
	contentChanged := oldHash != hand.ContentHash
	if contentChanged && hand.AnalyzeStatus == models.AnalyzeStatusDone {
		hand.AnalyzeStatus = models.AnalyzeStatusNone
	}

	// 洞察必须跟着一起清。画像读的是 review_insights，只重置 analyze_status 是不够的：
	// 留着旧洞察，画像会继续统计一手已经被改掉的手牌 —— 用户填错了牌再改，
	// 那手牌就以"改之前"的错误结论留在画像里；若改完又重新分析，更是新旧各计一份
	if err := config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(hand).Error; err != nil {
			return err
		}
		if !contentChanged {
			return nil
		}
		return deleteInsightsForHand(tx, userID, hand.ID)
	}); err != nil {
		return nil, err
	}
	return hand, nil
}

// DeleteHand 删除手牌（软删除）
func (s *ReviewService) DeleteHand(userID, handID uint) error {
	hand, err := s.GetHand(userID, handID)
	if err != nil {
		return err
	}
	return config.DB.Delete(hand).Error
}

// GetLeakTags 获取启用的漏洞标签字典（HTTP 响应用）
func (s *ReviewService) GetLeakTags() ([]dto.ReviewLeakTagResponse, error) {
	tags, err := s.GetLeakTagModels()
	if err != nil {
		return nil, err
	}

	list := make([]dto.ReviewLeakTagResponse, 0, len(tags))
	for i := range tags {
		list = append(list, dto.ToReviewLeakTagResponse(&tags[i]))
	}
	return list, nil
}

// GetLeakTagModels 获取启用的标签（模型形态）。
// 分析服务要拿它拼提示词、并做 tagCode 白名单校验，所以需要模型而不是响应 DTO
func (s *ReviewService) GetLeakTagModels() ([]models.ReviewLeakTag, error) {
	var tags []models.ReviewLeakTag
	if err := config.DB.Where("is_active = ?", true).
		Order("sort_order ASC, id ASC").Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

// loadGameNames 批量取场次名，避免列表里逐条查库
func (s *ReviewService) loadGameNames(hands []models.ReviewHand) map[uint]string {
	ids := make([]uint, 0, len(hands))
	seen := make(map[uint]bool, len(hands))
	for i := range hands {
		if hands[i].GameID == nil || seen[*hands[i].GameID] {
			continue
		}
		seen[*hands[i].GameID] = true
		ids = append(ids, *hands[i].GameID)
	}
	if len(ids) == 0 {
		return map[uint]string{}
	}

	var games []models.Game
	if err := config.DB.Select("id, name").Where("id IN ?", ids).Find(&games).Error; err != nil {
		// 场次名只是展示信息，取不到不该让整个列表失败
		return map[uint]string{}
	}

	names := make(map[uint]string, len(games))
	for _, g := range games {
		names[g.ID] = g.Name
	}
	return names
}

// checkGameAccessible 关联场次时校验用户确实是该场次参与者，
// 避免手牌被挂到任意 game_id 上
func (s *ReviewService) checkGameAccessible(userID uint, gameID *uint) error {
	if gameID == nil {
		return nil
	}

	var count int64
	err := config.DB.Model(&models.UserGame{}).
		Where("user_id = ? AND game_id = ?", userID, *gameID).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("只能关联你参与过的场次")
	}
	return nil
}

// applyHandRequest 把请求字段搬到模型上
func applyHandRequest(hand *models.ReviewHand, req *dto.ReviewHandRequest) {
	hand.GameID = req.GameID
	hand.Title = strings.TrimSpace(req.Title)
	hand.TableSize = req.TableSize
	hand.HeroPosition = req.HeroPosition
	hand.HeroCards = req.HeroCards
	hand.HeroStackBB = req.HeroStackBB
	hand.Stakes = strings.TrimSpace(req.Stakes)
	hand.SmallBlindBB = req.SmallBlindBB
	hand.BigBlindBB = req.BigBlindBB
	hand.AnteBB = req.AnteBB
	hand.Board = req.Board
	hand.VillainCount = req.VillainCount
	hand.Villains = req.Villains
	hand.PotType = req.PotType
	hand.Streets = req.Streets
	hand.HeroThought = strings.TrimSpace(req.HeroThought)
	hand.Result = req.Result
	hand.ResultAmount = req.ResultAmount
	hand.HeroTags = req.HeroTags
}

// buildHandTitle 用户没填标题时生成一个可读的默认标题
func buildHandTitle(hand *models.ReviewHand) string {
	parts := make([]string, 0, 3)
	if hand.HeroPosition != "" {
		parts = append(parts, hand.HeroPosition)
	}
	if hand.HeroCards != "" {
		parts = append(parts, hand.HeroCards)
	}
	if hand.PotType == models.PotTypeMulti {
		parts = append(parts, "多人池")
	} else if hand.PotType == models.PotTypeHU {
		parts = append(parts, "单挑")
	}
	return strings.Join(parts, " ")
}

// escapeLike 转义 LIKE 的特殊字符，让用户输入按字面量匹配
func escapeLike(s string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return replacer.Replace(s)
}
