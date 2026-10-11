-- ============================================================
-- 迁移：新增轻量埋点表 telemetry_events
-- 日期：2026-10-09
-- ============================================================
--
-- 背景
--   没有邮箱/手机号绑定，用户可以注册多个账号各拿一份免费点数。这确实是成本漏洞，
--   但**先别急着堵** —— 在"切换账号"这个行为里藏着最强的需求信号：
--   用户愿意付出注册新号 + 重新录手牌的摩擦，说明他要的量比你给的多。
--
--   所以先**记下来、量出来**，而不是拦掉：
--     * 有几个用户拥有 >1 个账号
--     * 有多少账号点数耗尽后，同 IP 冒出了第二个
--   第 2 个数比"日活"更能说明问题 —— 它测的是饥饿度。
--
--   等到办完主体能收钱的那天，"你点数用完了"就是转化率最高的时刻。
--   现在先把信号测出来，别浪费。
--
-- 两张表的关系
--   telemetry_events  记"谁在什么时候、从哪个 IP 做了什么"
--   user_ai_quotas    记"还剩多少点"（见 migrate_20261009_ai_point_quota.sql）
--   两者 JOIN 起来就是"同 IP 的多个账号、各自还剩多少" —— 最典型的白嫖画像
--
-- ============================================================
-- ⚠️ 用 ip 列之前，先把这两列的区别搞清楚
-- ============================================================
--   ip         取 gin 的 ClientIP()。它是否等于真实客户端，**完全取决于
--              TRUSTED_PROXIES 配得对不对**：
--                * 配对了 → 真实客户端 IP
--                * 配错/没配代理 → 可能是 nginx 的地址（所有人同一个值）
--                * 代理信任范围开太大 → 可能是客户端伪造的 X-Forwarded-For
--   remote_ip  取 TCP 对端，**内核给的，不可伪造**。代价是反代之后它是代理的地址
--
--   → 分析"同 IP 多账号"时**两列一起看**：ip 是猜测，remote_ip 是事实。
--     两列都指向"少数几个 IP / 少数几个用户"时，结论才可信
--
--   让 ip 变准需要两处配合，缺一不可：
--     1. 后端：TRUSTED_PROXIES 设成 nginx 的地址（默认已是 127.0.0.1,::1，
--        覆盖 nginx 与后端同机的常见形态）
--     2. nginx：proxy_set_header X-Real-IP $remote_addr;
--               proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
--    只做 1 不做 2 → ip 会等于 nginx 的地址（所有人一个值）
--    只做 2 不做 1 → 若 TRUSTED_PROXIES 为空，ip 会等于 nginx 的地址
--
--   开发环境：Vite 代理默认不转发 XFF（`xfwd` 默认关），所以本地开发时
--   ip 会是 127.0.0.1 —— 这是预期的，不是 bug
--
-- ⚠️ 本表**只记录、不拦截**，不参与任何请求的放行判断。要风控请另做限流
-- ⚠️ IP + UA 属于个人信息，上真实用户前必须写进隐私政策
-- ⚠️ 表模型在 call-auth 与 call-back 各有一份（两个独立 Go 模块），改一处要改两处
-- ⚠️ **建表归 call-back**（AutoMigrate 在它的 main.go 里）。call-auth 只写不自建 ——
--    两个服务同时对一张表 AutoMigrate 会互相覆盖列定义，与 users 表同一条规矩
--
-- 执行顺序
--   1) 重启 call-back —— AutoMigrate 建出这张表
--   2) 重启 call-auth —— 它只做一次"表在不在"的自检，缺了就告警（不报错、不建表）
--   3) 再执行本脚本确认、并跑下面的分析查询
--
--   本脚本**不建表**（DDL 一律交给 AutoMigrate），只做确认与查询，可重复执行。
--
-- 执行方式
--   mysql -h <host> -u <user> -p <db> < database/migrate_20261009_telemetry.sql

-- ---------- 1) 确认建表已完成 ----------
SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT, COLUMN_COMMENT
  FROM information_schema.COLUMNS
 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'telemetry_events'
 ORDER BY ORDINAL_POSITION;

-- ---------- 2) 总览：各类事件记了多少、覆盖多少用户与 IP ----------
SELECT event,
       COUNT(*)                  AS events,
       COUNT(DISTINCT user_id)   AS users,
       COUNT(DISTINCT NULLIF(ip, '')) AS ips,
       MIN(created_at)           AS first_at,
       MAX(created_at)           AS last_at
  FROM telemetry_events
 GROUP BY event;

-- ============================================================
-- 3) 你要的那两个数
-- ============================================================

-- 3.1 有几个用户拥有 >1 个账号（同一 IP 注册了多个账号）
--
-- remote_ips 是交叉验证：如果它们全是同一个值（比如 nginx 的地址），说明 ip 那列
-- 此时不可信（代理没配对），这时候真正说明问题的是 remote_ips 而不是 ip
SELECT ip,
       COUNT(DISTINCT NULLIF(remote_ip, ''))            AS distinct_remote_ips,
       GROUP_CONCAT(DISTINCT remote_ip)                 AS remote_ips,
       COUNT(DISTINCT user_id)                          AS accounts,
       GROUP_CONCAT(DISTINCT user_id ORDER BY user_id)  AS user_ids,
       MIN(created_at)                                  AS first_at,
       MAX(created_at)                                  AS last_at
  FROM telemetry_events
 WHERE event = 'user.register' AND ip <> ''
 GROUP BY ip
HAVING accounts > 1
 ORDER BY accounts DESC, last_at DESC;

-- 3.2 这些多账号来自几个 IP（一句话的结论）
SELECT COUNT(*) AS ips_with_multiple_accounts
  FROM (
    SELECT ip FROM telemetry_events
     WHERE event = 'user.register' AND ip <> ''
     GROUP BY ip HAVING COUNT(DISTINCT user_id) > 1
  ) x;

-- 3.3 **最关键的一张表**：同 IP 的多个账号，各自还剩多少点。
--     点数都接近 0 且同 IP = 最典型的"切账号白嫖"
SELECT t.ip,
       t.user_id,
       COALESCE(q.points, 9) AS points_left,
       MIN(t.created_at)     AS registered_at
  FROM telemetry_events t
  LEFT JOIN user_ai_quotas q ON q.user_id = t.user_id
 WHERE t.event = 'user.register' AND t.ip <> ''
   AND t.ip IN (
     SELECT ip FROM telemetry_events
      WHERE event = 'user.register' AND ip <> ''
      GROUP BY ip HAVING COUNT(DISTINCT user_id) > 1
   )
 ORDER BY t.ip, t.created_at;

-- ============================================================
-- 4) 消耗画像（按 IP / 按类型）
-- ============================================================

-- 4.1 每个 IP 烧掉多少点。单 IP 异常高 = 要么是团伙，要么是 NAT 后的真实用户
--     （国内移动网络出口 IP 是共享的，判IP前先想想这一条）
SELECT ip,
       COUNT(DISTINCT NULLIF(remote_ip, ''))                               AS distinct_remote_ips,
       COUNT(*)                                                            AS calls,
       COUNT(DISTINCT user_id)                                             AS users,
       SUM(CAST(JSON_UNQUOTE(JSON_EXTRACT(extra, '$.cost')) AS UNSIGNED))  AS points_burned
  FROM telemetry_events
 WHERE event = 'ai.call' AND ip <> ''
 GROUP BY ip
 ORDER BY points_burned DESC;

-- 4.2 按消耗类型汇总：哪块功能最费钱
SELECT JSON_UNQUOTE(JSON_EXTRACT(extra, '$.kind'))                         AS kind,
       COUNT(*)                                                            AS calls,
       COUNT(DISTINCT user_id)                                             AS users,
       SUM(CAST(JSON_UNQUOTE(JSON_EXTRACT(extra, '$.cost')) AS UNSIGNED))  AS points_burned
  FROM telemetry_events
 WHERE event = 'ai.call'
 GROUP BY kind
 ORDER BY points_burned DESC;

-- 4.3 单用户消耗排行：谁用得最狠（也是最可能付费的人）
SELECT user_id,
       COUNT(*)                                                            AS calls,
       SUM(CAST(JSON_UNQUOTE(JSON_EXTRACT(extra, '$.cost')) AS UNSIGNED))  AS points_burned
  FROM telemetry_events
 WHERE event = 'ai.call'
 GROUP BY user_id
 ORDER BY points_burned DESC
 LIMIT 20;
