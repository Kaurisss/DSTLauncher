$ErrorActionPreference = 'Stop'

Push-Location $PSScriptRoot
try {
    if (-not (Test-Path -LiteralPath 'web/node_modules')) {
        & npm.cmd --prefix web ci
        if ($LASTEXITCODE -ne 0) {
            throw "Frontend dependency installation failed (exit $LASTEXITCODE)."
        }
    }

    & npm.cmd --prefix web run build
    if ($LASTEXITCODE -ne 0) {
        throw "Frontend build failed (exit $LASTEXITCODE)."
    }

    & go build -o dst-windows.exe .
    if ($LASTEXITCODE -ne 0) {
        throw "Go build failed (exit $LASTEXITCODE)."
    }

    Write-Host "Built: $(Join-Path $PSScriptRoot 'dst-windows.exe')"
} finally {
    Pop-Location
}
