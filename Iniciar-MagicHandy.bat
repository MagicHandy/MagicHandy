@echo off
setlocal
cd /d "%~dp0"

if not exist "%~dp0scripts\start_stack.ps1" (
    echo ERRO: scripts\start_stack.ps1 nao encontrado.
    exit /b 1
)

rem Clique duplo: relanca minimizado e fecha este console na hora.
if /i "%~1"=="--worker" goto :worker

start "MagicHandy" /min "%~f0" --worker
exit /b 0

:worker
title MagicHandy - Iniciar
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\start_stack.ps1"
exit /b %errorlevel%
