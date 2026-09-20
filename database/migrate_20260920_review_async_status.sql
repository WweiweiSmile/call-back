-- ============================================================
-- 迁移：追问对话与画像总结改为异步，两张表增加任务状态列
-- 适用：M5（追问对话）之后的库 → 支持异步追问与异步总结重写的版本
-- 日期：2026-09-20
-- ============================================================
--
-- 背景
--   追问与画像总结原先是同步接口：HTTP 请求里当场调模型。K3 这类「始终推理」
--   模型一次要跑七八分钟（实测一次分析 464 秒），同步接口必然被前端或网关先掐断。
--   现改为与分析手牌一致的异步模式：先落一条 pending 记录并立即返回，后台
--   goroutine 调模型，前端轮询记录状态。为此需要：
--     * review_messages 增加 status / error_msg —— 只有 assistant 的占位行会处于
--       pending/running，user 行落库即 done
--     * review_profiles 增加 summary_status / summary_error / summary_started_at ——
--       画像行本身长期存在，异步的只是"重写总结"这一件事
--
--   为什么单独要 summary_started_at：判"这条还在跑吗"不能用 UpdatedAt ——
--   GET /profile 每次都会重算统计并 Save，用户每刷一次画像页就把 UpdatedAt 顶到
--   当下，时间窗永远不过期，总结在这台机器上再也跑不起来
--
-- 执行顺序（重要）
--   1) 先重启后端：AutoMigrate 会加上这五列。MySQL 在加列时会把存量行填成 GORM
--      tag 上声明的默认值（status='done'、summary_status='done'），
--      与"老对话都已完成、老画像没有正在重写的总结"的语义一致。
--      重启时 main.go 还会跑一次 ReapInterruptedTasks()，把上次退出时留下的
--      pending/running 行判死 —— 那些行的后台 goroutine 已经随进程消失了。
--   2) 再执行本脚本做确认与兜底回填。
--
--   本脚本**不做加列**（加列一律交给 AutoMigrate），只做确认与回填，可重复执行。
--
-- 执行方式
--   mysql -h <host> -u <user> -p <db> < database/migrate_20260920_review_async_status.sql

-- ---------- 1) 确认加列已完成 ----------
-- 期望看到 5 行；若为空说明后端还没重启过，先重启再回来
SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, COLUMN_DEFAULT, IS_NULLABLE, COLUMN_COMMENT
  FROM information_schema.COLUMNS
 WHERE TABLE_SCHEMA = DATABASE()
   AND (
        (TABLE_NAME = 'review_messages' AND COLUMN_NAME IN ('status', 'error_msg'))
     OR (TABLE_NAME = 'review_profiles' AND COLUMN_NAME IN ('summary_status', 'summary_error', 'summary_started_at'))
   )
 ORDER BY TABLE_NAME, COLUMN_NAME;

-- ---------- 2) 兜底回填 ----------
-- 正常应当是 0 行（加列时默认值已经填好）。留着是防"有人手工加列时漏了默认值"，
-- 那种情况下前端会把所有老对话当成"正在思考"而永久禁用输入框。
-- 带 WHERE 条件，重复执行无副作用。
UPDATE review_messages SET status = 'done' WHERE status IS NULL OR status = '';
UPDATE review_profiles SET summary_status = 'done' WHERE summary_status IS NULL OR summary_status = '';

-- ---------- 3) 校验 ----------
-- 期望：全部是 done；出现 pending/running 说明有待处理的任务或没清干净的残留
SELECT status, COUNT(*) AS cnt FROM review_messages GROUP BY status ORDER BY cnt DESC;
SELECT summary_status, COUNT(*) AS cnt FROM review_profiles GROUP BY summary_status ORDER BY cnt DESC;

-- 期望：0 行。这两张表都不该有非终态的任务
SELECT COUNT(*) AS inflight_messages FROM review_messages WHERE status IN ('pending', 'running');
SELECT COUNT(*) AS inflight_summaries FROM review_profiles WHERE summary_status IN ('pending', 'running');

-- 参考信息（**只统计，不回填**）：老代码在模型返回空内容时也会写一条 done 的
-- assistant 消息。这些行会被提示词的配对规则静默丢掉（它要求成对的问答），
-- 属于历史事实，不篡改
SELECT COUNT(*) AS done_but_empty_assistant
  FROM review_messages
 WHERE role = 'assistant' AND status = 'done' AND content = '';
