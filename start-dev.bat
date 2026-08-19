@echo off
setlocal EnableExtensions

cd /d "%~dp0"
set "POKERNODE_ROOT=%CD%"

echo.
echo   PokerNode Development Launcher
echo   ==============================
echo.

where go >nul 2>nul
if errorlevel 1 goto missing_go

where pnpm >nul 2>nul
if errorlevel 1 goto missing_pnpm

if not exist ".env" call :generate_env
if errorlevel 1 goto env_failed

for /f "usebackq eol=# delims=" %%L in (".env") do set "%%L"

if not defined POKERNODE_SESSION_SECRET goto invalid_session
if not defined POKERNODE_ENCRYPTION_KEY goto invalid_key
if not defined DATABASE_URL goto invalid_database

powershell -NoProfile -Command "try { if ([Convert]::FromBase64String($env:POKERNODE_ENCRYPTION_KEY).Length -ne 32) { exit 1 } } catch { exit 1 }"
if errorlevel 1 goto invalid_key

if not exist "web\node_modules" call :install_web
if errorlevel 1 goto install_failed

powershell -NoProfile -Command "if (Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue) { exit 1 }"
if errorlevel 1 goto backend_port_busy

powershell -NoProfile -Command "if (Get-NetTCPConnection -LocalPort 5173 -State Listen -ErrorAction SilentlyContinue) { exit 1 }"
if errorlevel 1 goto frontend_port_busy

echo [START] Backend: http://127.0.0.1:8080
start "PokerNode Backend" /D "%POKERNODE_ROOT%" cmd.exe /k "go run ./cmd/pokernode"

echo [START] Frontend: http://127.0.0.1:5173
start "PokerNode Frontend" /D "%POKERNODE_ROOT%\web" cmd.exe /k "pnpm dev --host 127.0.0.1"

echo [WAIT] Opening the acceptance page...
powershell -NoProfile -Command "Start-Sleep -Seconds 4"
start "" "http://127.0.0.1:5173"

echo.
echo [READY] PokerNode is running.
echo         Frontend: http://127.0.0.1:5173
echo         Backend:  http://127.0.0.1:8080
echo         Press Ctrl+C in each service window to stop it.
echo.
pause
exit /b 0

:generate_env
echo [SETUP] Creating .env with random local secrets...
powershell -NoProfile -ExecutionPolicy Bypass -Command "try { $key = New-Object byte[] 32; $session = New-Object byte[] 48; $rng = [Security.Cryptography.RandomNumberGenerator]::Create(); try { $rng.GetBytes($key); $rng.GetBytes($session) } finally { $rng.Dispose() }; $lines = @('POKERNODE_ADDR=127.0.0.1:8080','DATABASE_URL=postgres://pokernode:change-me@127.0.0.1:5432/pokernode?sslmode=disable',('POKERNODE_SESSION_SECRET=' + [Convert]::ToBase64String($session)),('POKERNODE_ENCRYPTION_KEY=' + [Convert]::ToBase64String($key))); [IO.File]::WriteAllLines((Join-Path $PWD '.env'), $lines, [Text.UTF8Encoding]::new($false)) } catch { Write-Error $_; exit 1 }"
exit /b %errorlevel%

:install_web
echo [SETUP] Installing frontend dependencies...
pushd "web"
call pnpm install
set "POKERNODE_INSTALL_RESULT=%errorlevel%"
popd
exit /b %POKERNODE_INSTALL_RESULT%

:missing_go
echo [ERROR] Go was not found. Install Go 1.25 or newer.
goto failed

:missing_pnpm
echo [ERROR] pnpm was not found. Run: corepack enable
goto failed

:env_failed
echo [ERROR] Could not create .env.
goto failed

:invalid_session
echo [ERROR] .env is missing POKERNODE_SESSION_SECRET.
goto failed

:invalid_key
echo [ERROR] POKERNODE_ENCRYPTION_KEY must decode to exactly 32 bytes.
goto failed

:invalid_database
echo [ERROR] .env is missing DATABASE_URL.
goto failed

:install_failed
echo [ERROR] Frontend dependency installation failed.
goto failed

:backend_port_busy
echo [ERROR] Port 8080 is already in use. Stop the existing process first.
goto failed

:frontend_port_busy
echo [ERROR] Port 5173 is already in use. Stop the existing process first.
goto failed

:failed
echo.
pause
exit /b 1
