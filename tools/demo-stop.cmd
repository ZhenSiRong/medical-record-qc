@echo off
cd /d "%~dp0.."
set "PGCTL=tools\pg\bin\pg_ctl.exe"
set "PGDATA=data\pgdata"

echo.
echo ========== 停止演示环境 ==========
echo.
echo [1/3] 停止前端静态服务 ...
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":8090 " ^| findstr LISTENING') do taskkill /F /PID %%a >nul 2>&1
echo       [OK]

echo [2/3] 停止后端服务 ...
taskkill /F /IM mrs.exe >nul 2>&1
echo       [OK]

echo [3/3] 停止 PostgreSQL ...
"%PGCTL%" -D "%PGDATA%" stop -m fast >nul 2>&1
echo       [OK]

echo.
echo ========== 已全部停止 ==========
echo.
pause
