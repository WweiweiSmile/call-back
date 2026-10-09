-- ============================================================
-- 迁移：AI 额度从「每日次数」改为「每人累计点数」
-- 适用：BYOK 版本 → 服务端 Key + 点数版本
-- 日期：2026-10-09
-- ============================================================
--
-- 背景
--   原先 AI 分析是 BYOK：每个用户必须自己配一个大模型 API Key 才能用。
--   目标用户是普通玩家，他们不会配、也听不懂"API Key"—— 这是一道所有真实
--   用户都过不去的门槛。现改为：服务端配一把兜底 Key（AI_SERVER_API_KEY），
--   所有用户开箱即用；同时用**每人累计点数**兜住成本。
--
--   额度从"每日 30 次、由历史行数算出"变成"每人一个点数余额、存在新表里"：
--     * 新表 user_ai_quotas，一列 points（默认 9 = 3 次复盘）
--     * 单价：复盘 3 / 对手画像 3 / 画像总结(手动) 3 / 追问 1（见 config/ai.go）
--     * 用完由运营手工改 points 充值，没有发放 UI —— "谁主动来要更多"本身就是
--       最强的需求信号，这比问卷准
--
--   为什么不加列到 user_preferences：那张表有硬约定「每个 service 只写自己的列」，
--   且 PreferenceService / AISettingService 都是**整行 Save**。额度要在热路径上
--   高频原子自减（UPDATE ... WHERE points >= ?），第三个写入方与整行 Save 会互相覆盖
--
-- 执行顺序（重要）
--   1) 先在 .env 配好 AI_SERVER_API_KEY（真 Key 只放 .env，.env.example 留空）
--   2) 重启后端：AutoMigrate 建出 user_ai_quotas 表。
--      注意**不会**给已存在的用户预建行 —— 行由首次扣点惰性创建，读不到时按 9 兜底。
--      这是刻意的：用户在认证中心侧创建，后端不做跨仓注册钩子
--   3) 再执行本脚本做确认与兜底回填
--
--   本脚本**不建表**（DDL 一律交给 AutoMigrate），只做确认与回填，可重复执行。
--
-- 执行方式
--   mysql -h <host> -u <user> -p <db> < database/migrate_20261009_ai_point_quota.sql

-- ---------- 1) 确认建表已完成 ----------
-- 期望看到 5 行；为空说明后端还没重启过，先重启再回来
SELECT COLUMN_NAME, COLUMN_TYPE, COLUMN_DEFAULT, IS_NULLABLE, COLUMN_COMMENT
  FROM information_schema.COLUMNS
 WHERE TABLE_SCHEMA = DATABASE()
   AND TABLE_NAME = 'user_ai_quotas'
 ORDER BY ORDINAL_POSITION;

-- ---------- 2) 兜底回填 ----------
-- 只补 NULL（手工建表可能留下 NULL 的 points）。
-- **不碰 0** —— 0 是合法的"已耗尽"状态，改它等于白送额度
UPDATE user_ai_quotas SET points = 9 WHERE points IS NULL;

-- 确认没有剩余 NULL
SELECT COUNT(*) AS null_points FROM user_ai_quotas WHERE points IS NULL;

-- ---------- 2.5) 给还没有额度行的老用户补一行 ----------
-- 接口在**读**的时候也会惰性补行（AIQuotaService.GetPoints），这里显式补一次是
-- 为了让运营**立刻**能在库里看到并调整每个人的余额，不必等谁先打开一次页面
--
-- 幂等，可重复执行
INSERT INTO user_ai_quotas (user_id, points, created_at, updated_at)
SELECT u.id, 9, NOW(3), NOW(3)
  FROM users u
 WHERE u.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM user_ai_quotas q WHERE q.user_id = u.id);

-- 确认活着的用户都有行了（users_without_quota 应为 0）
SELECT
  (SELECT COUNT(*) FROM users WHERE deleted_at IS NULL)                AS users_alive,
  (SELECT COUNT(*) FROM user_ai_quotas)                                AS quota_rows,
  (SELECT COUNT(*) FROM users u
    WHERE u.deleted_at IS NULL
      AND NOT EXISTS (SELECT 1 FROM user_ai_quotas q WHERE q.user_id = u.id)) AS users_without_quota;

-- ---------- 3) 现状一览 ----------
SELECT
  COUNT(*)                                  AS users_with_quota,
  COALESCE(SUM(points), 0)                  AS total_points_left,
  COALESCE(MIN(points), 0)                  AS min_points,
  COALESCE(MAX(points), 0)                  AS max_points,
  SUM(CASE WHEN points = 0 THEN 1 ELSE 0 END) AS exhausted_users
FROM user_ai_quotas;

-- ---------- 4) 运营充值配方（手动执行，不是迁移的一部分）----------
-- 给某个用户 +30 点（没有行就建一条）：
--   INSERT INTO user_ai_quotas (user_id, points) VALUES (<uid>, 30)
--     ON DUPLICATE KEY UPDATE points = points + 30;
--
-- 直接把某人设成指定值：
--   INSERT INTO user_ai_quotas (user_id, points) VALUES (<uid>, 9)
--     ON DUPLICATE KEY UPDATE points = 9;
--
-- 查某人还剩多少：
--   SELECT user_id, points FROM user_ai_quotas WHERE user_id = <uid>;
--
-- 注意：读不到行时接口按 9 兜底，所以"没有行"等价于"还有 9 点"
