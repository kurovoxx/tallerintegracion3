param(
    [string]$Kubectl = 'kubectl',
    [string]$Namespace = 'student-brojas',
    [switch]$RestartAuth
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$envFile = Join-Path $projectRoot '.env'
$smtpPasswordLine = Get-Content -LiteralPath $envFile |
    Where-Object { $_ -match '^SMTP_PASSWORD\s*=' } | Select-Object -Last 1
if (-not $smtpPasswordLine) { throw 'Configura SMTP_PASSWORD en el .env privado.' }
$smtpPassword = (($smtpPasswordLine -split '=', 2)[1]).Trim().Trim('"').Trim("'") -replace '\s', ''
if (-not $smtpPassword) { throw 'SMTP_PASSWORD está vacío.' }

# Validate the database target before any mutation; never apply this to Supabase.
$secretJson = & $Kubectl -n $Namespace get secret ti3-secrets -o json
if ($LASTEXITCODE -ne 0) { throw 'No se pudo leer ti3-secrets.' }
$secret = $secretJson | ConvertFrom-Json
$databaseURL = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($secret.data.DATABASE_URL))
if (([Uri]$databaseURL).Host -notin @('postgres', "postgres.$Namespace.svc.cluster.local")) {
    throw 'DATABASE_URL no apunta al servicio PostgreSQL de Pillán; no se aplicaron cambios.'
}

# Replace with resourceVersion to preserve every existing key and detect races.
# Secrets stay in memory and stdin; they never enter command arguments or logs.
$smtp = @{
    SMTP_HOST = 'smtp.gmail.com'
    SMTP_PORT = '587'
    SMTP_USERNAME = 'sigmaacademy.noreply@gmail.com'
    SMTP_PASSWORD = $smtpPassword
}
foreach ($name in $smtp.Keys) {
    $encoded = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($smtp[$name]))
    $secret.data | Add-Member -NotePropertyName $name -NotePropertyValue $encoded -Force
}
$secret | ConvertTo-Json -Depth 30 -Compress | & $Kubectl -n $Namespace replace -f -
if ($LASTEXITCODE -ne 0) { throw 'No se pudo actualizar SMTP en Pillán.' }

Get-Content -Raw -LiteralPath (Join-Path $projectRoot 'back/auth/db/migrations/001_password_resets.sql') |
    & $Kubectl -n $Namespace exec -i deployment/postgres -- psql -U postgres -d postgres -v ON_ERROR_STOP=1
if ($LASTEXITCODE -ne 0) { throw 'No se pudo aplicar la migración de recuperación.' }

& $Kubectl -n $Namespace exec deployment/postgres -- psql -U postgres -d postgres -tAc "SELECT to_regclass('identity.password_resets') IS NOT NULL"
if ($LASTEXITCODE -ne 0) { throw 'Falló la verificación de la migración.' }
if ($RestartAuth) {
    & $Kubectl -n $Namespace rollout restart deployment/auth
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo reiniciar Auth.' }
}
Write-Output 'SMTP y tabla de recuperación configurados en PostgreSQL de Pillán.'
