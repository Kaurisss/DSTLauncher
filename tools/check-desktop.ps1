param(
    [string]$Exe = (Join-Path $PSScriptRoot '../dst-desktop.exe')
)

$ErrorActionPreference = 'Stop'
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class DesktopCheck {
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode)]
    public static extern IntPtr LoadLibraryEx(string path, IntPtr file, uint flags);
    [DllImport("kernel32.dll")]
    public static extern IntPtr FindResource(IntPtr module, IntPtr name, IntPtr type);
    [DllImport("kernel32.dll")]
    public static extern bool FreeLibrary(IntPtr module);
    [DllImport("user32.dll")]
    public static extern IntPtr GetWindowDpiAwarenessContext(IntPtr hwnd);
    [DllImport("user32.dll")]
    public static extern bool AreDpiAwarenessContextsEqual(IntPtr first, IntPtr second);
    [DllImport("user32.dll")]
    public static extern uint GetDpiForWindow(IntPtr hwnd);
    [DllImport("user32.dll")]
    public static extern IntPtr SendMessage(IntPtr hwnd, uint message, IntPtr wparam, ref Rect rect);
    [DllImport("user32.dll")]
    public static extern bool GetWindowRect(IntPtr hwnd, out Rect rect);
    [DllImport("user32.dll")]
    static extern IntPtr SetThreadDpiAwarenessContext(IntPtr context);
    public static bool CheckDpiResize(IntPtr hwnd) {
        // WM_DPICHANGED supplies physical pixels; disable virtualization in the test caller too.
        IntPtr previous = SetThreadDpiAwarenessContext(new IntPtr(-4));
        try {
            Rect suggested, actual;
            if (!GetWindowRect(hwnd, out suggested)) return false;
            suggested.Right -= 16;
            suggested.Bottom -= 16;
            uint dpi = GetDpiForWindow(hwnd);
            SendMessage(hwnd, 0x02E0, new IntPtr(dpi | (dpi << 16)), ref suggested);
            return GetWindowRect(hwnd, out actual) && suggested.Left == actual.Left &&
                suggested.Top == actual.Top && suggested.Right == actual.Right && suggested.Bottom == actual.Bottom;
        } finally {
            SetThreadDpiAwarenessContext(previous);
        }
    }
    [DllImport("user32.dll", EntryPoint = "GetClassLongPtrW")]
    static extern IntPtr GetClassLong64(IntPtr hwnd, int index);
    [DllImport("user32.dll", EntryPoint = "GetClassLongW")]
    static extern uint GetClassLong32(IntPtr hwnd, int index);
    public static IntPtr WindowIcon(IntPtr hwnd) {
        return IntPtr.Size == 8 ? GetClassLong64(hwnd, -14) : new IntPtr(GetClassLong32(hwnd, -14));
    }
    [StructLayout(LayoutKind.Sequential)]
    public struct Rect { public int Left, Top, Right, Bottom; }
}
'@

$module = [DesktopCheck]::LoadLibraryEx((Resolve-Path $Exe).Path, [IntPtr]::Zero, 2)
if ($module -eq [IntPtr]::Zero) { throw 'Cannot load executable resources.' }
try {
    $hasIcon = [DesktopCheck]::FindResource($module, [IntPtr]1, [IntPtr]14) -ne [IntPtr]::Zero
    $hasManifest = [DesktopCheck]::FindResource($module, [IntPtr]1, [IntPtr]24) -ne [IntPtr]::Zero
    Write-Host "Embedded icon: $hasIcon; manifest: $hasManifest"
} finally {
    [void][DesktopCheck]::FreeLibrary($module)
}

$dataDir = Join-Path $env:TEMP ('dst-desktop-check-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $dataDir | Out-Null
$stdout = Join-Path $dataDir 'stdout.log'
$stderr = Join-Path $dataDir 'stderr.log'
$app = Start-Process -FilePath $Exe -ArgumentList @('-port', '0', '-data-dir', ('"' + $dataDir + '"')) -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
try {
    $deadline = [datetime]::UtcNow.AddSeconds(30)
    do {
        Start-Sleep -Milliseconds 100
        $app.Refresh()
        if ($app.HasExited) { throw "Desktop exited before opening a window: $($app.ExitCode). $(Get-Content $stderr -Raw)" }
        $ready = (Get-Content $stdout -Raw) -match 'Desktop window ready'
    } while ((-not $ready -or $app.MainWindowHandle -eq 0) -and [datetime]::UtcNow -lt $deadline)
    if (-not $ready -or $app.MainWindowHandle -eq 0) { throw 'Desktop window did not finish initializing.' }
    $context = [DesktopCheck]::GetWindowDpiAwarenessContext($app.MainWindowHandle)
    $perMonitorV2 = [DesktopCheck]::AreDpiAwarenessContextsEqual($context, [IntPtr](-4))
    Write-Host "Window uses Per-Monitor V2: $perMonitorV2"
    if ([DesktopCheck]::WindowIcon($app.MainWindowHandle) -eq [IntPtr]::Zero) { throw 'Window icon is missing.' }
    $dpi = [DesktopCheck]::GetDpiForWindow($app.MainWindowHandle)
    if (-not [DesktopCheck]::CheckDpiResize($app.MainWindowHandle)) {
        throw 'Window did not apply the physical rectangle suggested by WM_DPICHANGED.'
    }
    Write-Host "Window icon and DPI resize handling passed (current DPI=$dpi)."
    if (-not $app.CloseMainWindow()) { throw 'Cannot close desktop window.' }
    if (-not $app.WaitForExit(30000)) { throw 'Desktop did not exit after closing its window.' }
    if ($app.ExitCode -ne 0) { throw "Desktop exit failed: $($app.ExitCode)" }
    if (-not ($hasIcon -and $hasManifest -and $perMonitorV2)) {
        throw 'Desktop icon or DPI verification failed.'
    }
    Write-Host 'Desktop resource, DPI awareness and window-close checks passed.'
} finally {
    if (-not $app.HasExited) { $app.Kill(); $app.WaitForExit() }
    Write-Host "Test data: $dataDir"
}
