@echo off
rem Hentikan dan lepas ARUS print-agent (konfigurasi & log di %LOCALAPPDATA%\ArusPrintAgent ikut dihapus).
taskkill /IM print-agent.exe /F >nul 2>&1
del "%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup\ARUS Print Agent.lnk" >nul 2>&1
rmdir /S /Q "%LOCALAPPDATA%\ArusPrintAgent" >nul 2>&1
echo ARUS print-agent sudah dihapus.
pause
