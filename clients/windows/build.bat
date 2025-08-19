@echo off
echo Building OMS Windows Client...

REM Install dependencies
pip install -r requirements.txt

REM Build executable
pyinstaller --onefile --windowed --name "OMS Trading Client" --icon=icon.ico oms-client.py

echo Build complete! Check the dist folder for the executable.
pause