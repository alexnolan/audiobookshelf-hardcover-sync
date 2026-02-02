# Run this script as Administrator to install the AudiobookshelfHardcoverSync service

$serviceName = "AudiobookshelfHardcoverSync"
$exePath = "C:\vscode\github\audiobookshelf-hardcover-sync\bin\audiobookshelf-hardcover-sync.exe"
$workingDir = "C:\vscode\github\audiobookshelf-hardcover-sync"
$configPath = "C:\vscode\github\audiobookshelf-hardcover-sync\config.yaml"

# Install the service
Write-Host "Installing service..." -ForegroundColor Green
nssm install $serviceName $exePath "--server-only"

# Set the working directory
Write-Host "Setting working directory..." -ForegroundColor Green
nssm set $serviceName AppDirectory $workingDir

# Set environment variables
Write-Host "Setting environment variables..." -ForegroundColor Green
nssm set $serviceName AppEnvironmentExtra "CONFIG_PATH=$configPath"

# Set service to start automatically
Write-Host "Configuring service startup..." -ForegroundColor Green
nssm set $serviceName Start SERVICE_AUTO_START

# Set service description
Write-Host "Setting service description..." -ForegroundColor Green
nssm set $serviceName Description "Syncs Audiobookshelf library with Hardcover"

# Set stdout/stderr log files
Write-Host "Configuring logging..." -ForegroundColor Green
$logDir = "$workingDir\logs"
New-Item -ItemType Directory -Force -Path $logDir | Out-Null
nssm set $serviceName AppStdout "$logDir\service.log"
nssm set $serviceName AppStderr "$logDir\service-error.log"

# Set log rotation
nssm set $serviceName AppRotateFiles 1
nssm set $serviceName AppRotateOnline 1
nssm set $serviceName AppRotateBytes 10485760  # 10MB

Write-Host "`nService installed successfully!" -ForegroundColor Green
Write-Host "Service Name: $serviceName" -ForegroundColor Cyan
Write-Host "Executable: $exePath" -ForegroundColor Cyan
Write-Host "Working Directory: $workingDir" -ForegroundColor Cyan
Write-Host "Port: 8765" -ForegroundColor Cyan
Write-Host "`nTo start the service, run:" -ForegroundColor Yellow
Write-Host "  Start-Service $serviceName" -ForegroundColor White
Write-Host "`nTo check service status:" -ForegroundColor Yellow
Write-Host "  Get-Service $serviceName" -ForegroundColor White
Write-Host "`nWeb UI will be available at:" -ForegroundColor Yellow
Write-Host "  http://localhost:8765" -ForegroundColor White
