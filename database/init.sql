-- 创建数据库
CREATE DATABASE IF NOT EXISTS call_game DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE call_game;

-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(100) NOT NULL UNIQUE COMMENT '用户名',
    nickname VARCHAR(100) COMMENT '昵称',
    avatar VARCHAR(500) COMMENT '头像URL',
    status VARCHAR(20) DEFAULT 'active' COMMENT '状态: active-正常, inactive-禁用',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL,
    INDEX idx_status (status),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户表';

-- 游戏场次表
CREATE TABLE IF NOT EXISTS games (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL COMMENT '游戏名称',
    description TEXT COMMENT '游戏描述',
    creator_id BIGINT UNSIGNED NOT NULL COMMENT '创建者ID',
    status VARCHAR(20) DEFAULT '' COMMENT '状态: ''-未结束, ended-已结束',
    start_time DATETIME NULL COMMENT '开始时间',
    end_time DATETIME NULL COMMENT '结束时间',
    player_count INT DEFAULT 0 COMMENT '当前人数',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_creator_id (creator_id),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='游戏场次表';

-- 用户-场次关联表
CREATE TABLE IF NOT EXISTS user_games (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
    game_id BIGINT UNSIGNED NOT NULL COMMENT '场次ID',
    joined_at DATETIME NOT NULL COMMENT '加入时间',
    left_at DATETIME NULL COMMENT '退出时间',
    status VARCHAR(20) DEFAULT 'active' COMMENT '状态: active-活跃, left-已退出',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL,
    UNIQUE INDEX idx_user_game (user_id, game_id),
    INDEX idx_game_id (game_id),
    INDEX idx_status (status),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户-场次关联表';

-- 存取分记录表
CREATE TABLE IF NOT EXISTS transactions (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
    game_id BIGINT UNSIGNED NOT NULL COMMENT '场次ID',
    operator_id BIGINT UNSIGNED NOT NULL COMMENT '操作人ID',
    operator_type VARCHAR(20) NOT NULL COMMENT '操作类型: self-自主操作, proxy-代理操作',
    trans_type VARCHAR(20) NOT NULL COMMENT '交易类型: deposit-存分, withdraw-取分',
    amount BIGINT NOT NULL COMMENT '数量',
    balance_after BIGINT NOT NULL COMMENT '操作后余额',
    remark TEXT COMMENT '备注',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL,
    INDEX idx_user_id (user_id),
    INDEX idx_game_id (game_id),
    INDEX idx_operator_id (operator_id),
    INDEX idx_created_at (created_at),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='存取分记录表';

-- 场次余额表
CREATE TABLE IF NOT EXISTS user_balances (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
    game_id BIGINT UNSIGNED NOT NULL COMMENT '场次ID',
    total_deposit BIGINT DEFAULT 0 COMMENT '场次存分总量',
    total_withdraw BIGINT DEFAULT 0 COMMENT '场次取分总量',
    current_balance BIGINT DEFAULT 0 COMMENT '场次当前余额',
    last_trans_time DATETIME NULL COMMENT '最后交易时间',
    balance_status VARCHAR(20) DEFAULT 'balanced' COMMENT '平衡状态: balanced-平衡, unbalanced-不平衡',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL,
    UNIQUE INDEX idx_user_game_balance (user_id, game_id),
    INDEX idx_game_id (game_id),
    INDEX idx_balance_status (balance_status),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='场次余额表';

-- 复盘手牌表
-- streets / villains / hero_tags 用 JSON 列：手牌是聚合根，永远整手读写，
-- 不存在"只查某条 action"的场景，拆表只会让每次读取都要 join 组装
CREATE TABLE IF NOT EXISTS review_hands (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL COMMENT '记录者ID，数据隔离依据',
    game_id BIGINT UNSIGNED NULL COMMENT '关联场次ID，可为空',
    title VARCHAR(255) COMMENT '标题，为空时后端按位置+底牌生成',
    hero_position VARCHAR(10) COMMENT '我的位置: UTG/MP/CO/BTN/SB/BB',
    hero_cards VARCHAR(8) COMMENT '我的底牌，规范格式如 AsKh',
    hero_stack_bb DOUBLE DEFAULT 0 COMMENT '我的有效筹码(BB)',
    stakes VARCHAR(20) COMMENT '盲注级别，如 5/10',
    board VARCHAR(10) COMMENT '公共牌，按发牌顺序拼接如 Qs7h2d3c9s',
    villain_count INT DEFAULT 0 COMMENT '对手数量',
    villains JSON COMMENT '对手信息 [{position, stackBb, isKey}]',
    pot_type VARCHAR(10) DEFAULT 'hu' COMMENT 'hu-单挑, multi-多人池',
    streets JSON COMMENT '按街的行动序列 [{street, actions, potStartBb}]',
    hero_thought TEXT COMMENT '我当时是怎么想的',
    result VARCHAR(10) DEFAULT 'unknown' COMMENT 'win/lose/fold/unknown',
    result_amount DOUBLE NULL COMMENT '输赢金额(BB)',
    hero_tags JSON COMMENT '用户自打标签',
    content_hash VARCHAR(64) COMMENT '内容指纹，未变更则不重复调用AI',
    analyze_status VARCHAR(20) DEFAULT 'none' COMMENT 'none/pending/done/failed',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL,
    INDEX idx_rh_user_created (user_id, created_at),
    INDEX idx_game_id (game_id),
    INDEX idx_hero_position (hero_position),
    INDEX idx_analyze_status (analyze_status),
    INDEX idx_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='复盘手牌表';

-- 漏洞标签字典表
-- 存在的意义：让 AI 只能从受控词表里选标签，否则"翻前跟注过宽"和"翻前跟注范围太松"
-- 会作为两个不同字符串存下来，永远聚不了合，用户画像就退化了
-- 字典内容由 services 启动时按 code upsert（见 main.go 的 seedLeakTags）
CREATE TABLE IF NOT EXISTS review_leak_tags (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code VARCHAR(64) NOT NULL COMMENT '唯一标识，AI 引用它',
    name VARCHAR(64) NOT NULL COMMENT '中文名',
    category VARCHAR(20) COMMENT 'preflop/postflop/mental/bankroll',
    description VARCHAR(255) COMMENT '判定说明，会写进提示词帮模型选对标签',
    is_active TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否启用',
    sort_order INT DEFAULT 0 COMMENT '排序',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX idx_code (code),
    INDEX idx_category (category)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='复盘漏洞标签字典表';

-- 插入测试用户
INSERT INTO users (username, nickname, status) VALUES 
('testuser1', '测试用户1', 'active'),
('testuser2', '测试用户2', 'active'),
('testuser3', '测试用户3', 'active');
