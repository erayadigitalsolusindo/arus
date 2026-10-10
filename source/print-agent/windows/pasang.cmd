@echo off
rem Pasang ARUS print-agent untuk pengguna Windows ini: salin program ke %LOCALAPPDATA%\ArusPrintAgent,
rem buat pintasan di folder Startup (berjalan otomatis setiap login), lalu jalankan sekarang.
rem Jalankan dari folder yang berisi print-agent.exe (dan opsional print-agent.json).
setlocal
set "DEST=%LOCALAPPDATA%\ArusPrintAgent"
if not exist "%~dp0print-agent.exe" (
  echo print-agent.exe tidak ditemukan di folder ini.
  pause
  exit /b 1
)
taskkill /IM print-agent.exe /F >nul 2>&1
if not exist "%DEST%" mkdir "%DEST%"
copy /Y "%~dp0print-agent.exe" "%DEST%\" >nul
if exist "%~dp0print-agent.json" if not exist "%DEST%\print-agent.json" copy /Y "%~dp0print-agent.json" "%DEST%\" >nul
powershell -NoProfile -Command "$s=(New-Object -ComObject WScript.Shell).CreateShortcut([Environment]::GetFolderPath('Startup')+'\ARUS Print Agent.lnk'); $s.TargetPath='%DEST%\print-agent.exe'; $s.WorkingDirectory='%DEST%'; $s.Save()"
start "" "%DEST%\print-agent.exe"
echo.
echo ARUS print-agent terpasang di %DEST% dan sudah berjalan.
echo Konfigurasi: %DEST%\print-agent.json  (isi "origins" dengan alamat aplikasi ARUS; "printer" kosong = printer default)
echo Log: %DEST%\print-agent.log
pause
