@echo off
setlocal

rem Use this script to run the program locally on Windows.
cd /d "%~dp0" || exit /b 1

if not exist "build\" mkdir "build" || exit /b 1

go build -o "build\gocode.exe" ".\src" || exit /b 1

"build\gocode.exe" %*
exit /b %errorlevel%
