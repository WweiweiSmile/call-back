package services

import (
	"call-go/config"
	"call-go/dto"
	"call-go/models"
	"errors"
	"strings"

	"gorm.io/gorm"
)

// OpponentSearchDefaultLimit / MaxLimit 搜索返回条数。下拉框用不着太多，
// 上限也顺便挡住"一次拉全表"这种用法
const (
	OpponentSearchDefaultLimit = 20
	OpponentSearchMaxLimit     = 50
)

// OpponentStatsMaxHands 量化统计最多覆盖多少手。
//
// 设上限是为了不让一个高频对手把整张表拉进内存。到 200 手这个量级时，
// 更早的手牌对"他现在怎么打"的参考价值也已经很低了 —— 而且统计口径
// 越老越可能对应一个已经变了的打法
const OpponentStatsMaxHands = 200

// OpponentService 对手名单。按 user_id 隔离，与 review_hands 同一套口径
type OpponentService struct{}

// GetOpponent 取当前用户的某个对手。
// 带 user_id 过滤：别人的对手对当前用户表现为"不存在"，不泄露存在性
func (s *OpponentService) GetOpponent(userID, opponentID uint) (*models.Opponent, error) {
	var opponent models.Opponent
	err := config.DB.Where("id = ? AND user_id = ?", opponentID, userID).First(&opponent).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("对手不存在")
		}
		return nil, err
	}
	return &opponent, nil
}

// ListHandsForOpponent 分页取与某对手交手的手牌，按时间倒序
func (s *OpponentService) ListHandsForOpponent(userID, opponentID uint, page, pageSize int) ([]models.ReviewHand, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 两次都重新构造查询：同一个 *gorm.DB 上先 Count 再 Find 会残留
	// SELECT count(*) 的语句状态，多花的这点代价换一个不会咬人的写法
	var total int64
	if err := s.handsByOpponentQuery(userID, opponentID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	hands := make([]models.ReviewHand, 0, pageSize)
	err := s.handsByOpponentQuery(userID, opponentID).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&hands).Error
	if err != nil {
		return nil, 0, err
	}
	return hands, total, nil
}

// LoadHandsForStats 取该对手最近 OpponentStatsMaxHands 手牌，供量化统计使用。
//
// 统计要覆盖**全部**交手手牌，不能只统计列表当前页 —— 否则翻一页数字就变一次
func (s *OpponentService) LoadHandsForStats(userID, opponentID uint) ([]models.ReviewHand, error) {
	hands := make([]models.ReviewHand, 0, 64)
	err := s.handsByOpponentQuery(userID, opponentID).
		Order("created_at DESC").
		Limit(OpponentStatsMaxHands).
		Find(&hands).Error
	if err != nil {
		return nil, err
	}
	return hands, nil
}

// handsByOpponentQuery 与某对手交手的手牌查询。
//
// 手牌与对手的关联藏在 villains 这个 JSON 列里的 opponentId，用 JSON_CONTAINS 现查。
// 不另建关联表：对手只是手牌的一部分，拆表会让"整手读写"的手牌多一次 join，
// 而这点数据量根本不需要那种优化
func (s *OpponentService) handsByOpponentQuery(userID, opponentID uint) *gorm.DB {
	return config.DB.Model(&models.ReviewHand{}).
		Where("user_id = ? AND villains IS NOT NULL", userID).
		Where("JSON_CONTAINS(villains, JSON_OBJECT('opponentId', ?))", opponentID)
}

// SearchOpponents 按名字模糊搜当前用户的对手，附带"已交手 N 手"。
//
// 交手数用 JSON_CONTAINS 现算，不额外维护计数列：计数列要在
// 增删改手牌四处都记得同步，漏一处就长期不准，而这里的数据量根本不需要那种优化。
//
// 注意它只数得到 M7.1 之后的手牌 —— 老手牌的 villains 里没有 opponentId，
// 认不出是哪位对手。下拉里的数字偏小是预期内的，不是 bug
func (s *OpponentService) SearchOpponents(userID uint, keyword string, limit int) ([]dto.OpponentResponse, error) {
	if limit <= 0 || limit > OpponentSearchMaxLimit {
		limit = OpponentSearchDefaultLimit
	}

	list := make([]dto.OpponentResponse, 0, limit)
	query := config.DB.Table("opponents AS o").
		Select(`o.id AS id, o.name AS name,
			(SELECT COUNT(*) FROM review_hands AS h
			 WHERE h.user_id = o.user_id
			   AND h.deleted_at IS NULL
			   AND h.villains IS NOT NULL
			   AND JSON_CONTAINS(h.villains, JSON_OBJECT('opponentId', o.id))) AS hand_count`).
		Where("o.user_id = ? AND o.deleted_at IS NULL", userID)

	if kw := strings.TrimSpace(keyword); kw != "" {
		query = query.Where("o.name LIKE ?", "%"+escapeLike(kw)+"%")
	}

	if err := query.Order("hand_count DESC, o.id DESC").Limit(limit).Scan(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ResolveOpponents 把对手名解析成对手表的 id：没有就建一条。
//
// 名字是唯一事实来源，VillainInfo.OpponentID 只是解析结果，所以这里会覆盖
// 客户端传来的值（校验层已经把它清零了）。没有名字的对手（老数据）不关联。
func (s *OpponentService) ResolveOpponents(userID uint, villains []models.VillainInfo) error {
	for i := range villains {
		name := strings.TrimSpace(villains[i].Name)
		if name == "" {
			villains[i].OpponentID = 0
			continue
		}
		opponent, err := s.findOrCreate(userID, name)
		if err != nil {
			return err
		}
		villains[i].OpponentID = opponent.ID
	}
	return nil
}

// findOrCreate 按 (user_id, 名字) 查一个人，查不到就建。
// 比对时忽略大小写：同一个人写成 "Tom" 和 "tom" 不该变成两条记录
func (s *OpponentService) findOrCreate(userID uint, name string) (*models.Opponent, error) {
	opponent, err := s.findByName(userID, name)
	if err == nil {
		// 用户这次换了种写法（大小写/空格），以这次的为准，
		// 否则下拉里会按旧写法显示，用户会以为没搜到
		if opponent.Name != name {
			if err := config.DB.Model(opponent).Update("name", name).Error; err != nil {
				return nil, err
			}
			opponent.Name = name
		}
		return opponent, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	created := &models.Opponent{UserID: userID, Name: name}
	if err := config.DB.Create(created).Error; err != nil {
		// 并发提交同一手牌时可能已被另一个请求建好，回查一次再定成败
		if again, retryErr := s.findByName(userID, name); retryErr == nil {
			return again, nil
		}
		return nil, err
	}
	return created, nil
}

// findByName 按名字查对手（忽略大小写）
func (s *OpponentService) findByName(userID uint, name string) (*models.Opponent, error) {
	var opponent models.Opponent
	if err := config.DB.
		Where("user_id = ? AND LOWER(name) = ?", userID, strings.ToLower(name)).
		First(&opponent).Error; err != nil {
		return nil, err
	}
	return &opponent, nil
}
