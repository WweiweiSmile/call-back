package controllers

import (
	"call-go/dto"
	"call-go/middleware"
	"call-go/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type TransactionController struct {
	transactionService *services.TransactionService
}

func NewTransactionController() *TransactionController {
	return &TransactionController{
		transactionService: &services.TransactionService{},
	}
}

// Deposit 存分
func (c *TransactionController) Deposit(ctx *gin.Context) {
	var req dto.DepositRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	userID := middleware.GetUserID(ctx)

	transaction, err := c.transactionService.Deposit(userID, &req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(transaction))
}

// Withdraw 取分
func (c *TransactionController) Withdraw(ctx *gin.Context) {
	var req dto.WithdrawRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	userID := middleware.GetUserID(ctx)

	transaction, err := c.transactionService.Withdraw(userID, &req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(transaction))
}

// GetTransactionList 获取交易记录列表
func (c *TransactionController) GetTransactionList(ctx *gin.Context) {
	gameID, err := strconv.ParseUint(ctx.Param("game_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的游戏ID"))
		return
	}

	userIDStr := ctx.Query("user_id")
	var userID uint
	if userIDStr != "" {
		uid, _ := strconv.ParseUint(userIDStr, 10, 32)
		userID = uint(uid)
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))

	list, err := c.transactionService.GetTransactionList(userID, uint(gameID), page, pageSize)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("获取交易记录失败: "+err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(list))
}

// GetUserBalance 获取用户余额
func (c *TransactionController) GetUserBalance(ctx *gin.Context) {
	gameID, err := strconv.ParseUint(ctx.Param("game_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的游戏ID"))
		return
	}

	userID := middleware.GetUserID(ctx)

	balance, err := c.transactionService.GetUserBalance(userID, uint(gameID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("获取用户余额失败: "+err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(balance))
}

// GetGameParticipants 获取游戏参与者列表（含余额）
func (c *TransactionController) GetGameParticipants(ctx *gin.Context) {
	gameID, err := strconv.ParseUint(ctx.Param("game_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的游戏ID"))
		return
	}

	participants, err := c.transactionService.GetGameParticipants(uint(gameID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("获取参与者列表失败: "+err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(participants))
}
