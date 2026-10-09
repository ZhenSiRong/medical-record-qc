@echo off
REM ============================================================
REM enable-wsl2.cmd v2 —— 一键启用 WSL2（自动提权 + 日志）
REM 双击运行 → UAC 点「是」→ 窗口停留执行 → 看到全部 OK 后可手动重启。
REM 日志：tools\enable-wsl2.log
REM ============================================================
set LOG=%~dp0enable-wsl2.log
net session >nul 2>&1
if %errorlevel% neq 0 (
  echo 正在请求管理员权限，请在 UAC 弹窗中点「是」...
  powershell -NoProfile -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
  exit /b
)

echo ===== WSL2 启用开始 %date% %time% ===== > "%LOG%"
echo [1/3] 启用「虚拟机平台」VirtualMachinePlatform ...
dism /online /enable-feature /featurename:VirtualMachinePlatform /all /norestart >> "%LOG%" 2>&1
echo [2/3] 确认「适用于 Linux 的 Windows 子系统」...
dism /online /enable-feature /featurename:Microsoft-Windows-Subsystem-Linux /all /norestart >> "%LOG%" 2>&1
echo [3/3] 尝试更新 WSL 内核（旧版报错可忽略） ...
wsl --update >> "%LOG%" 2>&1

echo.
echo ===== 完成，日志见 %LOG% ===== >> "%LOG%"
echo 执行完毕！本窗口可以关闭。请【手动重启电脑】，重启后运行：
echo     wsl --set-default-version 2
echo     wsl --install -d Ubuntu
pause
