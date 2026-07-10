@echo off
cd /d "%~dp0"
title MagicHandy - Parar
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\stop_stack.ps1"
exit /b %errorlevel%
