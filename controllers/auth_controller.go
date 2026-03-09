package controllers

import (
	"call-go/dto"
	"call-go/middleware"
	"call-go/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AuthController struct {
	authService *services.AuthService
}

func NewAuthController() *AuthController {
	return &AuthController{
		authService: &services.AuthService{},
	}
}

// Register 注册
func (c *AuthController) Register(ctx *gin.Context) {
	var req dto.RegisterRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	resp, err := c.authService.Register(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(resp))
}

// Login 登录
func (c *AuthController) Login(ctx *gin.Context) {
	var req dto.LoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse("参数错误: "+err.Error()))
		return
	}

	resp, err := c.authService.Login(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(resp))
}

// GetUserInfo 获取当前用户信息
func (c *AuthController) GetUserInfo(ctx *gin.Context) {
	userID := middleware.GetUserID(ctx)

	resp, err := c.authService.GetUserInfo(userID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, dto.ErrorResponse(err.Error()))
		return
	}

	ctx.JSON(http.StatusOK, dto.SuccessResponse(resp))
}
