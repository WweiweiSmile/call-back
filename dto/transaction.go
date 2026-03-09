package dto

import "time"

// DepositRequest 存分请求
type DepositRequest struct {
	GameID    uint   `json:"game_id" binding:"required"`
	TargetUserID *uint `json:"target_user_id"` // 代理操作时需要
	Amount    int64  `json:"amount" binding:"required,min=1"`
	Remark    string `json:"remark"`
}

// WithdrawRequest 取分请求
type WithdrawRequest struct {
	GameID    uint   `json:"game_id" binding:"required"`
	TargetUserID *uint `json:"target_user_id"` // 代理操作时需要
	Amount    int64  `json:"amount" binding:"required,min=1"`
	Remark    string `json:"remark"`
}

// TransactionResponse 交易记录响应
type TransactionResponse struct {
	ID           uint      `json:"id"`
	UserID       uint      `json:"user_id"`
	UserName     string    `json:"user_name,omitempty"`
	GameID       uint      `json:"game_id"`
	OperatorID   uint      `json:"operator_id"`
	OperatorName string    `json:"operator_name,omitempty"`
	OperatorType string    `json:"operator_type"`
	TransType    string    `json:"trans_type"`
	Amount       int64     `json:"amount"`
	BalanceAfter int64     `json:"balance_after"`
	Remark       string    `json:"remark"`
	CreatedAt    time.Time `json:"created_at"`
}

// TransactionListResponse 交易记录列表响应
type TransactionListResponse struct {
	Total int64                  `json:"total"`
	List  []TransactionResponse  `json:"list"`
}

// UserBalanceResponse 用户余额响应
type UserBalanceResponse struct {
	UserID         uint   `json:"user_id"`
	UserName       string `json:"user_name,omitempty"`
	GameID         uint   `json:"game_id"`
	TotalDeposit   int64  `json:"total_deposit"`
	TotalWithdraw  int64  `json:"total_withdraw"`
	CurrentBalance int64  `json:"current_balance"`
	BalanceStatus  string `json:"balance_status"`
}
