$ErrorActionPreference = 'Stop'

Push-Location $PSScriptRoot
try {
    if (-not (Test-Path -LiteralPath 'web/node_modules')) {
        & pnpm --dir web install --frozen-lockfile
        if ($LASTEXITCODE -ne 0) {
            throw "Frontend dependency installation failed (exit $LASTEXITCODE)."
        }
    }

    & pnpm --dir web run build
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
