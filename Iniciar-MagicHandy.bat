@echo off
cd /d "%~dp0"
title MagicHandy - Iniciar
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\start_stack.ps1"
if errorlevel 1 (
    echo.
    echo ERRO ao iniciar o MagicHandy. Leia a mensagem acima.
    pause
    exit /b 1
)
echo.
echo MagicHandy em background. Abra http://127.0.0.1:49717
echo Para parar: Parar-MagicHandy.bat
timeout /t 8 /nobreak >nul
