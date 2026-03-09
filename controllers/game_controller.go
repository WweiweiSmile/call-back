package controllers

import (
	"call-go/dto"
	"call-go/middleware"
	"call-go/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type GameController struct {
	gameService *services.GameService
}

func NewGameController() *GameController {
	return &GameController{
		gameService: &services.GameService{},
	}
}

// CreateGame 创建游戏
func (c *GameController) CreateGame(ctx *gin.Context) {
	var req dto.CreateGameRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	userID := middleware.GetUserID(ctx)

	game, err := c.gameService.CreateGame(userID, &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("创建游戏失败: "+err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(game))
}

// GetGameList 获取游戏列表
func (c *GameController) GetGameList(ctx *gin.Context) {
	status := ctx.Query("status")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	userID := middleware.GetUserID(ctx)

	list, err := c.gameService.GetGameList(userID, status, page, pageSize)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("获取游戏列表失败: "+err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(list))
}

// GetGame 获取游戏详情
func (c *GameController) GetGame(ctx *gin.Context) {
	gameID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的游戏ID"))
		return
	}

	game, err := c.gameService.GetGame(uint(gameID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, dto.ErrorResponse("游戏不存在"))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(game))
}

// JoinGame 加入游戏
func (c *GameController) JoinGame(ctx *gin.Context) {
	var req dto.JoinGameRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	userID := middleware.GetUserID(ctx)

	err := c.gameService.JoinGame(userID, req.GameID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(nil))
}

// LeaveGame 退出游戏
func (c *GameController) LeaveGame(ctx *gin.Context) {
	gameID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的游戏ID"))
		return
	}

	userID := middleware.GetUserID(ctx)

	err = c.gameService.LeaveGame(userID, uint(gameID))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(nil))
}

// EndGame 结束游戏
func (c *GameController) EndGame(ctx *gin.Context) {
	gameID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("无效的游戏ID"))
		return
	}

	userID := middleware.GetUserID(ctx)

	err = c.gameService.EndGame(userID, uint(gameID))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(nil))
}

// GetMyGames 获取我的游戏
func (c *GameController) GetMyGames(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	userID := middleware.GetUserID(ctx)

	list, err := c.gameService.GetMyGames(userID, page, pageSize)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("获取我的游戏失败: "+err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(list))
}

// GetCreatedGames 获取我创建的游戏
func (c *GameController) GetCreatedGames(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	userID := middleware.GetUserID(ctx)

	list, err := c.gameService.GetCreatedGames(userID, page, pageSize)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse("获取我创建的游戏失败: "+err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(list))
}
