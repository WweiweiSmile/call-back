-- ============================================================
-- 迁移：清理「已被取代的旧分析」留下的洞察
-- 适用：本修复上线前的库（画像把同一手牌的新旧两套结论都统计了）
-- 日期：2026-09-17
-- ============================================================
--
-- 背景
--   ReviewMemoryService.RecordInsights 原先按 analysis_id 清旧洞察，于是
--   「填错手牌 → 改动 → 重新分析」会生成新的 analysis 行，旧行名下的洞察原样
--   留着，画像聚合时同一手牌被统计两次（错的那次也计入）。
--   代码已改成按 hand_id 清（见 services/review_memory.go 的 RecordInsights），
--   但线上已经产生的历史数据还在库里，本脚本负责清理。
--
-- 判据（与修复后的代码口径一致）
--   一手牌只保留一套洞察，来自「对它**当前内容**的那次分析」：
--     有效分析 = 该手牌下 status='done'、且 content_hash 与手牌当前 content_hash
--                相等的分析中，id 最大的那一条
--   洞察的 analysis_id 不是有效分析的一律删除。
--
--   为什么用「指纹相等」而不是「最新的那次分析」：content_hash 是手牌每次保存时
--   重算的，指纹对得上就等价于「这条分析跑的就是现在这份内容」。只取「最新」的话，
--   「改了手牌但还没重新分析」的那些手牌，其旧结论会被当成有效留下 —— 那正是要清掉的东西。
--
-- 已知副作用（有意为之）
--   2026-09-17 同时改过内容指纹算法（把不进提示词的 heroTags 从指纹里去掉），
--   所以修复上线前最后编辑过的那些手牌，其指纹与历史分析记录不再相等，
--   会被判为「当前内容没有对应分析」，洞察一并清理。这与新代码在编辑手牌时
--   就清洞察的行为一致。
--
-- 执行顺序
--   1) 先重启后端，确保跑的是修复后的代码（本次不需要加列，不重启也不会出错，
--      只是新数据仍会继续污染）
--   2) 先只执行【第 1 段 体检】和【第 2 段 预览】，确认数字与你预期相符
--   3) 确认无误后把第 3 段的 @confirm 从 0 改成 1，整体执行一次
--      脚本幂等，可重复执行
--
-- 执行方式
--   mysql -h <host> -u <user> -p <db> < database/migrate_20260917_purge_stale_insights.sql
--
-- 回滚
--   待删除的行已全量落在 review_insights_stale_20260917 里，按第 5 段回滚即可

-- ============================================================
-- 1) 体检：先看清问题规模（只读）
-- ============================================================

-- 1.1 同一手牌有多次成功分析的情况
SELECT h.id                                        AS hand_id,
       COUNT(*)                                    AS done_analyses,
       COUNT(DISTINCT a.content_hash)              AS hash_kinds,
       SUM(a.content_hash = h.content_hash)        AS matches_current
  FROM review_hands h
  JOIN review_analyses a ON a.hand_id = h.id AND a.status = 'done'
 GROUP BY h.id, h.content_hash
HAVING done_analyses > 1
 ORDER BY done_analyses DESC
 LIMIT 20;

-- 1.2 待删洞察的构成。三类互斥，加起来应当正好等于 Total stale insights；
--     对不上说明判据的边界有你没料到的情形，先别往下走
SELECT
    SUM(v.keep_id IS NOT NULL)                              AS superseded,
    SUM(v.keep_id IS NULL AND a.id IS NOT NULL)             AS content_changed,
    SUM(a.id IS NULL)                                       AS orphaned,
    COUNT(*)                                                AS total_stale
  FROM review_insights i
  LEFT JOIN review_analyses a ON a.id = i.analysis_id
  LEFT JOIN (
        -- 每手牌的有效分析：指纹匹配的成功分析中 id 最大的那条
        SELECT a2.hand_id, MAX(a2.id) AS keep_id
          FROM review_analyses a2
          JOIN review_hands h2 ON h2.id = a2.hand_id
         WHERE a2.status = 'done' AND a2.content_hash = h2.content_hash
         GROUP BY a2.hand_id
       ) v ON v.hand_id = i.hand_id
 WHERE v.keep_id IS NULL OR v.keep_id <> i.analysis_id;

-- 1.2.1 将被保留的洞察数
SELECT COUNT(*) AS kept
  FROM review_insights i
  JOIN (
        SELECT a2.hand_id, MAX(a2.id) AS keep_id
          FROM review_analyses a2
          JOIN review_hands h2 ON h2.id = a2.hand_id
         WHERE a2.status = 'done' AND a2.content_hash = h2.content_hash
         GROUP BY a2.hand_id
       ) v ON v.hand_id = i.hand_id AND v.keep_id = i.analysis_id;

-- 1.3 受影响的用户
SELECT COUNT(DISTINCT i.user_id) AS affected_users
  FROM review_insights i
 WHERE i.analysis_id NOT IN (
        SELECT MAX(a.id) FROM review_analyses a
          JOIN review_hands h ON h.id = a.hand_id
         WHERE a.status = 'done' AND a.content_hash = h.content_hash
         GROUP BY a.hand_id);

-- ============================================================
-- 2) 把待删的行全量搬进备份表（这一步是只读原表的）
--    备份表兼作回滚来源，删之前务必确认它写进去了
-- ============================================================

CREATE TABLE IF NOT EXISTS review_insights_stale_20260917 (
  insight_id  BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  user_id     BIGINT UNSIGNED NOT NULL,
  hand_id     BIGINT UNSIGNED NOT NULL,
  analysis_id BIGINT UNSIGNED NOT NULL,
  kind        VARCHAR(20)     NOT NULL,
  tag_code    VARCHAR(64)         NULL,
  severity    BIGINT              NULL,
  evidence    TEXT            NOT NULL,
  created_at  DATETIME(3)         NULL
);

-- 用 INSERT IGNORE 而不是「先 TRUNCATE 再写」：备份表按 insight_id 做主键，
-- 重复执行只会补进新出现的待删行，绝不会抹掉上一次执行留下的备份 ——
-- 那才是回滚唯一的来源
INSERT IGNORE INTO review_insights_stale_20260917
SELECT i.id, i.user_id, i.hand_id, i.analysis_id,
       i.kind, i.tag_code, i.severity, i.evidence, i.created_at
  FROM review_insights i
  LEFT JOIN (
        SELECT a2.hand_id, MAX(a2.id) AS keep_id
          FROM review_analyses a2
          JOIN review_hands h2 ON h2.id = a2.hand_id
         WHERE a2.status = 'done' AND a2.content_hash = h2.content_hash
         GROUP BY a2.hand_id
       ) v ON v.hand_id = i.hand_id
 WHERE v.keep_id IS NULL OR v.keep_id <> i.analysis_id;

-- 本次新增的备份行数，应当等于第 1.2 段的 total_stale（重复执行时为 0）
SELECT ROW_COUNT() AS backed_up_this_run;

-- 备份表累计行数。它跨多次执行累积，只会变大或不变，是回滚的全部依据
SELECT COUNT(*) AS backup_table_total FROM review_insights_stale_20260917;

-- ============================================================
-- 3) 删除（改 0 为 1 才会真的删）
-- ============================================================

SET @confirm := 0;

DELETE i
  FROM review_insights i
  JOIN review_insights_stale_20260917 s ON s.insight_id = i.id
 WHERE @confirm = 1;

-- 3.1 校验：删完之后不该再有「不属于有效分析」的洞察（期望 0）
SELECT COUNT(*) AS remaining_stale
  FROM review_insights i
  LEFT JOIN (
        SELECT a2.hand_id, MAX(a2.id) AS keep_id
          FROM review_analyses a2
          JOIN review_hands h2 ON h2.id = a2.hand_id
         WHERE a2.status = 'done' AND a2.content_hash = h2.content_hash
         GROUP BY a2.hand_id
       ) v ON v.hand_id = i.hand_id
 WHERE @confirm = 1
   AND (v.keep_id IS NULL OR v.keep_id <> i.analysis_id);

-- 3.2 画像里的 leaks / strengths 是洞察的聚合，下次打开画像页会自动重算
--     （GetProfile → RefreshProfile），不用管。
--     但 summary 是一段自然语言，只在阈值到了才重写，不会自己发现底下的数据变了 ——
--     清空 last_summary_at 让 ShouldRewriteSummary 立刻返回 true，
--     受影响的用户下次分析后就会重写总结（会产生一次模型调用）。
--     不希望动总结的话，把这一段注释掉。
UPDATE review_profiles
   SET last_summary_at = NULL
 WHERE @confirm = 1
   AND user_id IN (SELECT DISTINCT user_id FROM review_insights_stale_20260917);

-- ============================================================
-- 4) 收尾
--    确认无误后再执行；确认前先留着备份表，它是唯一的回滚来源
-- ============================================================

-- DROP TABLE review_insights_stale_20260917;

-- ============================================================
-- 5) 回滚（后悔药）
--    备份表还在的话，把所有被删的洞察原样写回
-- ============================================================

-- INSERT IGNORE INTO review_insights
--   (id, user_id, hand_id, analysis_id, kind, tag_code, severity, evidence, created_at)
-- SELECT insight_id, user_id, hand_id, analysis_id, kind, tag_code, severity, evidence, created_at
--   FROM review_insights_stale_20260917;
