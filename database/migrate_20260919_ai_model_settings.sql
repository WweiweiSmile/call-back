-- ============================================================
-- 迁移：user_preferences 增加 BYOK 模型配置四列
-- 适用：有 M5.5（盲注默认值）的库 → 支持「模型设置」的版本
-- 日期：2026-09-19
-- ============================================================
--
-- 背景
--   「模型设置」让用户自己填 BaseURL + 模型名 + 自己的 API Key。
--   这推翻了原先"API Key 仅存后端 .env"的设计，所以：
--     * 用户 Key 用 AES-256-GCM 加密后存 ai_api_key_encrypted，明文永不落库
--     * ai_api_key_hint 明文存末 4 位，只用于掩码回显 —— 主密钥轮换后
--       密文解不开，但仍要能告诉用户"你配过一把 Key"
--     * 服务端 env 里的 DEEPSEEK_API_KEY 自此不再对任何用户生效
--
-- 执行顺序（重要）
--   1) 先在 .env 里配好 PREF_ENCRYPTION_KEY（openssl rand -base64 32），
--      再重启后端：AutoMigrate 会加上这四列。
--      没有这把密钥时服务能正常启动，但「模型设置」只能读不能写。
--   2) 再执行本脚本做确认与状态核对。
--
--   本脚本**不做任何数据迁移**（原因见第 3 段），全部语句只读，可重复执行。
--
-- 执行方式
--   mysql -h <host> -u <user> -p <db> < database/migrate_20260919_ai_model_settings.sql

-- ---------- 1) 确认加列已完成 ----------
-- 期望 4 行：ai_base_url / ai_model / ai_api_key_encrypted / ai_api_key_hint
-- 若为空说明后端还没用新代码重启过，先重启再回来
SELECT COLUMN_NAME, COLUMN_TYPE, COLUMN_DEFAULT, IS_NULLABLE, COLUMN_COMMENT
  FROM information_schema.COLUMNS
 WHERE TABLE_SCHEMA = DATABASE()
   AND TABLE_NAME = 'user_preferences'
   AND COLUMN_NAME IN ('ai_base_url', 'ai_model', 'ai_api_key_encrypted', 'ai_api_key_hint')
 ORDER BY COLUMN_NAME;

-- ---------- 2) 盲注三列必须原样保留 ----------
-- 期望每行的三个值都没变（0.5 / 1 / 0 或用户自己设过的值）。
-- 这一条是给"加列把老数据写坏"这类事故留的核对点
SELECT user_id, small_blind_bb, big_blind_bb, ante_bb
  FROM user_preferences
 ORDER BY user_id;

-- ---------- 3) 不需要数据迁移，这是刻意的 ----------
--
-- 没有 backfill 语句，理由：
--   ai_base_url 为空串时，读取路径（services.AISettingService.resolveAIValues）
--   会回退到 config.DefaultAIPreset()，也就是 DEEPSEEK_BASE_URL / DEEPSEEK_MODEL
--   的当前值。
--
--   把这套默认值写死进每一行反而更差：将来部署方改了 DEEPSEEK_BASE_URL，
--   没主动配置过的用户会跟着走新地址；写死了就再也跟不动了。
--
--   所以升级后老用户打开「模型设置」看到的是"已经填好的地址与模型名"，
--   他们唯一要做的事就是粘一把自己的 Key。

-- ---------- 4) 升级后的状态核对 ----------
-- 期望"已配置 = 0"：刚升级完还没有人配过 Key
SELECT COUNT(*) AS 总行数,
       SUM(ai_api_key_encrypted <> '') AS 已配置,
       SUM(ai_base_url <> '') AS 已填地址
  FROM user_preferences;

-- ---------- 5) 密文抽查 ----------
-- 期望：ai_api_key_encrypted 是 base64 串，且**不含** sk- 之类的明文片段；
--      ai_api_key_hint 是 4 位尾号（没有配过则是空串）
SELECT user_id, ai_base_url, ai_model,
       LEFT(ai_api_key_encrypted, 12) AS 密文前缀,
       ai_api_key_hint
  FROM user_preferences
 ORDER BY user_id;
