# Run this script as Administrator to uninstall the AudiobookshelfHardcoverSync service

$serviceName = "AudiobookshelfHardcoverSync"

# Stop the service if running
Write-Host "Stopping service..." -ForegroundColor Yellow
try {
    Stop-Service $serviceName -ErrorAction SilentlyContinue
    Write-Host "Service stopped." -ForegroundColor Green
} catch {
    Write-Host "Service was not running." -ForegroundColor Gray
}

# Uninstall the service
Write-Host "Uninstalling service..." -ForegroundColor Yellow
nssm remove $serviceName confirm

Write-Host "`nService uninstalled successfully!" -ForegroundColor Green
