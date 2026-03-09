# Call-Go 后端项目

这是一个基于 Go 语言的游戏存取分管理系统后端。

## 项目结构

```
call-go/
├── main.go                 # 主程序入口
├── go.mod                  # Go 模块文件
├── go.sum                  # 依赖版本锁定文件
├── config/
│   └── database.go         # 数据库配置
├── models/                 # 数据模型
│   ├── user.go
│   ├── game.go
│   ├── user_game.go
│   ├── transaction.go
│   └── user_balance.go
├── dto/                    # 数据传输对象
│   ├── response.go
│   ├── game.go
│   └── transaction.go
├── services/               # 业务逻辑层
│   ├── game_service.go
│   └── transaction_service.go
├── controllers/            # 控制器层
│   ├── game_controller.go
│   └── transaction_controller.go
├── routes/                 # 路由配置
│   └── routes.go
└── database/
    └── init.sql            # 数据库初始化脚本
```

## 数据库配置

数据库信息：
- 地址：localhost:3306
- 账号：root
- 密码：Qw13101192533
- 数据库名：call_game

## 快速开始

### 1. 创建数据库

首先在 MySQL 中执行初始化脚本：

```bash
mysql -u root -p < database/init.sql
```

或者手动连接 MySQL 并执行 `database/init.sql` 文件中的内容。

### 2. 安装依赖

```bash
cd E:\Codes\call-go
go mod tidy
```

### 3. 运行项目

```bash
go run main.go
```

服务将在 `http://localhost:8080` 启动。

## API 接口文档

### 游戏相关接口

#### 创建游戏
```
POST /api/v1/games
Content-Type: application/json

{
  "name": "游戏名称",
  "description": "游戏描述",
  "start_time": "2026-03-08T20:00:00+08:00",
  "end_time": "2026-03-08T22:00:00+08:00"
}
```

#### 获取游戏列表
```
GET /api/v1/games?status=ongoing&page=1&page_size=10
```

#### 获取我的游戏
```
GET /api/v1/games/my?page=1&page_size=10
```

#### 获取我创建的游戏
```
GET /api/v1/games/created?page=1&page_size=10
```

#### 获取游戏详情
```
GET /api/v1/games/:id
```

#### 加入游戏
```
POST /api/v1/games/join
Content-Type: application/json

{
  "game_id": 1
}
```

#### 退出游戏
```
POST /api/v1/games/:id/leave
```

### 交易相关接口

#### 存分
```
POST /api/v1/transactions/deposit
Content-Type: application/json

{
  "game_id": 1,
  "target_user_id": 2,  // 可选，代理操作时需要
  "amount": 1000,
  "remark": "备注信息"
}
```

#### 取分
```
POST /api/v1/transactions/withdraw
Content-Type: application/json

{
  "game_id": 1,
  "target_user_id": 2,  // 可选，代理操作时需要
  "amount": 500,
  "remark": "备注信息"
}
```

#### 获取游戏交易记录
```
GET /api/v1/transactions/game/:game_id?user_id=1&page=1&page_size=20
```

#### 获取用户余额
```
GET /api/v1/transactions/balance/:game_id
```

#### 获取游戏参与者列表（含余额）
```
GET /api/v1/transactions/participants/:game_id
```

### 健康检查
```
GET /health
```

## 业务规则

1. **积分独立性**：每场游戏的积分完全独立，场次间积分不能转移
2. **操作权限**：
   - 用户自己可以操作自己的积分
   - 游戏创建者可以代理操作该游戏中任何参与者的积分
3. **平衡判断**：
   - 平衡：场次存分总量 - 场次取分总量 = 0
   - 不平衡：场次存分总量 - 场次取分总量 ≠ 0
4. **取分限制**：取分不能超过当前场次余额

## 开发说明

- 当前使用模拟用户ID（固定为1），实际项目中应该从 JWT token 或 session 中获取
- 建议添加用户认证中间件
- 建议添加日志系统
- 建议添加单元测试
