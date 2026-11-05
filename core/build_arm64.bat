@echo off
echo =======================================================
echo   正在交叉编译 Agnes Gateway Core (Android ARM64)
echo =======================================================
set CGO_ENABLED=0
set GOOS=android
set GOARCH=arm64
go build -ldflags="-s -w" -o agnes-gateway-arm64 .
if %ERRORLEVEL% equ 0 (
    echo [成功] 已生成 agnes-gateway-arm64
    if not exist "..\app\src\main\jniLibs\arm64-v8a" mkdir "..\app\src\main\jniLibs\arm64-v8a"
    copy /Y agnes-gateway-arm64 "..\app\src\main\jniLibs\arm64-v8a\libgateway.so"
    echo [成功] 已同步至 Android 工程 jniLibs: app\src\main\jniLibs\arm64-v8a\libgateway.so
) else (
    echo [失败] 编译失败，请检查 Go 环境
)
pause
