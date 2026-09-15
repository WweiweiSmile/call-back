-- ============================================================
-- 迁移：手牌复盘增加「几人桌」，位置词表随之扩展
-- 适用：M1~M3 的库 → 支持 table_size 的版本
-- 日期：2026-09-15
-- ============================================================
--
-- 背景
--   位置取值从此随人数变化（见 models/review_hand.go 与 utils/hand.go 的
--   positionsByTableSize）。原有的 MP 被废弃：它既能读成 LJ 也能读成 HJ，
--   两种读法对应的翻前范围差别很大，交给模型分析是实打实的歧义，故统一迁到 HJ。
--
-- 执行顺序（重要）
--   1) 先重启后端：AutoMigrate 会加上 table_size 列，MySQL 在加列时会把存量行
--      自动填成默认值 9，与「人数未填按满员桌」的语义一致。
--   2) 再执行本脚本，只需处理位置词表的数据迁移（第 2、3 段）。
--      第 4 段是兜底，防止有人手工加列时漏了默认值。
--
--   本脚本只做数据迁移，可重复执行（幂等）。
--
-- 执行方式
--   mysql -h <host> -u <user> -p <db> < database/migrate_20260915_table_size.sql

-- ---------- 1) 确认加列已完成 ----------
-- 期望看到 table_size 一行；若为空说明后端还没重启过，先重启再回来
SELECT COLUMN_NAME, COLUMN_TYPE, COLUMN_DEFAULT, IS_NULLABLE, COLUMN_COMMENT
  FROM information_schema.COLUMNS
 WHERE TABLE_SCHEMA = DATABASE()
   AND TABLE_NAME = 'review_hands'
   AND COLUMN_NAME = 'table_size';

-- ---------- 2) 我的手牌位置：MP → HJ ----------
UPDATE review_hands SET hero_position = 'HJ' WHERE hero_position = 'MP';

-- ---------- 3) 对手位置（JSON 列）：MP → HJ ----------
-- 只用 JSON_SEARCH 判存在，避免对不含 MP 的行做无谓的整列重写
UPDATE review_hands
   SET villains = REPLACE(villains, '"MP"', '"HJ"')
 WHERE JSON_SEARCH(villains, 'one', 'MP') IS NOT NULL;

-- ---------- 4) 人数兜底 ----------
UPDATE review_hands SET table_size = 9 WHERE table_size IS NULL OR table_size = 0;

-- ---------- 校验 ----------
-- 期望：不再出现 MP；人数全部落在 2~9
SELECT hero_position, COUNT(*) AS cnt FROM review_hands GROUP BY hero_position ORDER BY cnt DESC;
SELECT table_size, COUNT(*) AS cnt FROM review_hands GROUP BY table_size ORDER BY table_size DESC;

-- 期望：剩下这些行的对手位置里还残留 MP（正常应为 0 行）
SELECT id FROM review_hands WHERE JSON_SEARCH(villains, 'one', 'MP') IS NOT NULL;
