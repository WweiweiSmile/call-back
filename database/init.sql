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
    status VARCHAR(20) DEFAULT 'pending' COMMENT '状态: pending-即将开始, ongoing-进行中, ended-已结束',
    start_time DATETIME NULL COMMENT '开始时间',
    end_time DATETIME NULL COMMENT '结束时间',
    player_count INT DEFAULT 0 COMMENT '当前人数',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL,
    INDEX idx_creator_id (creator_id),
    INDEX idx_status (status),
    INDEX idx_deleted_at (deleted_at)
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

-- 插入测试用户
INSERT INTO users (username, nickname, status) VALUES 
('testuser1', '测试用户1', 'active'),
('testuser2', '测试用户2', 'active'),
('testuser3', '测试用户3', 'active');
