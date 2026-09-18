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

// OpponentService 对手名单。按 user_id 隔离，与 review_hands 同一套口径
type OpponentService struct{}

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
