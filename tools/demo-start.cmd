@echo off
cd /d "%~dp0.."
set "PGCTL=tools\pg\bin\pg_ctl.exe"
set "PGDATA=data\pgdata"
set "BACKEND=data\mrs.exe"

echo.
echo ========== 病案管理系统 · 启动演示环境 ==========
echo.

echo [1/3] 启动 PostgreSQL (5432) ...
"%PGCTL%" -D "%PGDATA%" -l "data\pg.log" -o "-p 5432" start >nul 2>&1
ping -n 5 127.0.0.1 >nul
netstat -ano | findstr ":5432 " | findstr LISTENING >nul
if errorlevel 1 (echo       [!] PostgreSQL 未启动，请查看 data\pg.log) else (echo       [OK] PostgreSQL 已就绪)

echo [2/3] 启动后端服务 (8080) ...
if not exist "%BACKEND%" (
  echo       首次运行，正在编译后端（约 15 秒）...
  pushd mrs-backend
  go build -buildvcs=false -o "..\data\mrs.exe" .
  popd
)
powershell -NoProfile -Command "Start-Process -FilePath '%CD%\data\mrs.exe' -WorkingDirectory '%CD%\mrs-backend' -WindowStyle Hidden" >nul 2>&1
ping -n 7 127.0.0.1 >nul
netstat -ano | findstr ":8080 " | findstr LISTENING >nul
if errorlevel 1 (echo       [!] 后端未启动，请查看 data\backend.err) else (echo       [OK] 后端服务已就绪)

echo [3/3] 启动前端静态服务 (8090) ...
set "PYEXE="
for /f "delims=" %%i in ('where python 2^>nul') do (
  if not defined PYEXE set "PYEXE=%%i"
)
if not defined PYEXE (
  echo       [!] 未找到 python，请先安装或加入 PATH
) else (
  powershell -NoProfile -Command "Start-Process -FilePath '%PYEXE%' -ArgumentList 'serve.py','8090' -WorkingDirectory '%CD%' -WindowStyle Hidden" >nul 2>&1
  ping -n 5 127.0.0.1 >nul
  netstat -ano | findstr ":8090 " | findstr LISTENING >nul
  if errorlevel 1 (echo       [!] 前端未启动) else (echo       [OK] 前端服务已就绪)
)

echo.
echo ================== 启动完成 ==================
echo.
echo   演示入口（浏览器打开）:
echo     http://127.0.0.1:8090/frontend/index.html
echo.
echo   登录账号:
echo     admin    / admin123   病案室管理员（全部权限）
echo     deptuser / 123456     科室医师（越权操作提示 403）
echo.
echo   其他地址:
echo     后端健康   http://127.0.0.1:8080/healthz
echo     规格说明书 http://127.0.0.1:8090/spec-preview.html
echo.
echo   停止服务: 双击 tools\demo-stop.cmd
echo =============================================
echo.
pause
