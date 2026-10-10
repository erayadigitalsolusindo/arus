@echo off
rem Cetak halaman uji langsung ke printer (tanpa aplikasi) untuk memastikan jalur RAW ke printer bekerja.
"%LOCALAPPDATA%\ArusPrintAgent\print-agent.exe" -test
echo Lihat hasil di printer. Bila tidak keluar, periksa %LOCALAPPDATA%\ArusPrintAgent\print-agent.log
pause
