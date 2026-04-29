$ErrorActionPreference = "Stop"

# lsp-cli.ps1 在 Windows release archive 内启动对应 exe，默认连接线上 gate。
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Bin = Join-Path $ScriptDir "lsp-cli_windows_amd64.exe"
if (-not (Test-Path $Bin)) {
    $Bin = Join-Path (Split-Path -Parent $ScriptDir) "lsp-cli_windows_amd64.exe"
}
if (-not (Test-Path $Bin)) {
    throw "未找到 lsp-cli_windows_amd64.exe"
}

$Ws = $env:LSP_CLI_WS
if ([string]::IsNullOrWhiteSpace($Ws)) {
    $Ws = "wss://racoo.cn/ws"
}

& $Bin --ws $Ws @args
