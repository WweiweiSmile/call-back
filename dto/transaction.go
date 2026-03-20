package dto

import "time"

// DepositRequest 存分请求
type DepositRequest struct {
	GameID       uint   `json:"gameId" binding:"required"`
	TargetUserID *uint  `json:"targetUserId"` // 代理操作时需要
	Amount       int64  `json:"amount" binding:"required,min=1"`
	Remark       string `json:"remark"`
}

// WithdrawRequest 取分请求
type WithdrawRequest struct {
	GameID       uint   `json:"gameId" binding:"required"`
	TargetUserID *uint  `json:"targetUserId"` // 代理操作时需要
	Amount       int64  `json:"amount" binding:"required,min=1"`
	Remark       string `json:"remark"`
}

// TransactionResponse 交易记录响应
type TransactionResponse struct {
	ID           uint      `json:"id"`
	UserID       uint      `json:"userId"`
	UserName     string    `json:"userName,omitempty"`
	GameID       uint      `json:"gameId"`
	OperatorID   uint      `json:"operatorId"`
	OperatorName string    `json:"operatorName,omitempty"`
	OperatorType string    `json:"operatorType"`
	TransType    string    `json:"transType"`
	Amount       int64     `json:"amount"`
	BalanceAfter int64     `json:"balanceAfter"`
	Remark       string    `json:"remark"`
	CreatedAt    time.Time `json:"createdAt"`
}

// TransactionListResponse 交易记录列表响应
type TransactionListResponse struct {
	Total int64                 `json:"total"`
	List  []TransactionResponse `json:"list"`
}

// UserBalanceResponse 用户余额响应
type UserBalanceResponse struct {
	UserID         uint   `json:"userId"`
	UserName       string `json:"userName,omitempty"`
	GameID         uint   `json:"gameId"`
	TotalDeposit   int64  `json:"totalDeposit"`
	TotalWithdraw  int64  `json:"totalWithdraw"`
	CurrentBalance int64  `json:"currentBalance"`
	BalanceStatus  string `json:"balanceStatus"`
}
