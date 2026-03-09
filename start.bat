@echo off
echo ====================================
echo Call-Go 后端服务启动脚本
echo ====================================
echo.

REM 刷新环境变量
echo [1/3] 刷新环境变量...
for /f "tokens=2*" %%A in ('reg query "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v Path') do set "SYSTEM_PATH=%%B"
for /f "tokens=2*" %%A in ('reg query "HKCU\Environment" /v Path') do set "USER_PATH=%%B"
set "PATH=%SYSTEM_PATH%;%USER_PATH%"

REM 检查 Go 是否可用
echo [2/3] 检查 Go 环境...
go version
if %errorlevel% neq 0 (
    echo 错误: 找不到 go 命令，请确保 Go 已正确安装并添加到环境变量
    pause
    exit /b 1
)

REM 启动服务
echo [3/3] 启动后端服务...
echo.
echo 服务将在 http://localhost:8080 启动
echo 按 Ctrl+C 停止服务
echo.
cd /d "%~dp0"
go run main.go

pause
