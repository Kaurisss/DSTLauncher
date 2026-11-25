param(
    [switch]$Desktop
)

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

    $output = 'dst-windows.exe'
    $buildArgs = @('build')
    if ($Desktop) {
        $targetOS = & go env GOOS
        if ($targetOS -ne 'windows') {
            throw 'Desktop builds require GOOS=windows.'
        }
        & go generate -tags desktop ./internal/desktopresources
        if ($LASTEXITCODE -ne 0) {
            throw "Desktop resource generation failed (exit $LASTEXITCODE)."
        }
        $output = 'dst-desktop.exe'
        $buildArgs += @('-tags', 'desktop', '-ldflags', '-H=windowsgui')
    }
    $buildArgs += @('-o', $output, '.')
    & go @buildArgs
    if ($LASTEXITCODE -ne 0) {
        throw "Go build failed (exit $LASTEXITCODE)."
    }

    Write-Host "Built: $(Join-Path $PSScriptRoot $output)"
} finally {
    Pop-Location
}
