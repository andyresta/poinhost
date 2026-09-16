# Menyiapkan jendela poinhost lalu menangkapnya ke PNG.
#
# -X/-Y (opsional) mengklik satu titik di dalam jendela lebih dulu, memakai
# koordinat relatif terhadap sudut kiri-atas jendela — dipakai untuk berpindah
# menu sebelum menangkap layar.
#
# PrintWindow dipanggil dengan PW_RENDERFULLCONTENT (0x2); tanpa flag itu
# jendela WebView2 tertangkap hitam karena isinya dirender komposit, bukan GDI.
param(
    [Parameter(Mandatory = $true)][string]$OutFile,
    [int]$X = -1,
    [int]$Y = -1,
    [int]$W = 1440,
    [int]$H = 900,
    [int]$WaitMs = 2500
)

Add-Type -AssemblyName System.Drawing

Add-Type @'
using System;
using System.Runtime.InteropServices;
public class W2 {
    [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint f);
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out R r);
    [DllImport("user32.dll")] public static extern bool MoveWindow(IntPtr h, int x, int y, int w, int t, bool re);
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
    [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int c);
    [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
    [DllImport("user32.dll")] public static extern void mouse_event(uint f, uint x, uint y, uint d, IntPtr e);
    [StructLayout(LayoutKind.Sequential)] public struct R { public int L, T, Rt, B; }
}
'@

$proc = Get-Process -Name "poinhost-dev" -ErrorAction SilentlyContinue |
    Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
if ($null -eq $proc) { Write-Output "ERR: jendela poinhost tidak ditemukan"; exit 1 }
$h = $proc.MainWindowHandle

[void][W2]::ShowWindow($h, 9)
[void][W2]::MoveWindow($h, 60, 40, $W, $H, $true)
[void][W2]::SetForegroundWindow($h)
Start-Sleep -Milliseconds 700

$r = New-Object W2+R
[void][W2]::GetWindowRect($h, [ref]$r)

if ($X -ge 0 -and $Y -ge 0) {
    [void][W2]::SetCursorPos(($r.L + $X), ($r.T + $Y))
    Start-Sleep -Milliseconds 250
    [W2]::mouse_event(0x0002, 0, 0, 0, [IntPtr]::Zero)   # LEFTDOWN
    [W2]::mouse_event(0x0004, 0, 0, 0, [IntPtr]::Zero)   # LEFTUP
    Start-Sleep -Milliseconds $WaitMs
}

[void][W2]::GetWindowRect($h, [ref]$r)
$w = $r.Rt - $r.L; $t = $r.B - $r.T
$bmp = New-Object System.Drawing.Bitmap $w, $t
$g = [System.Drawing.Graphics]::FromImage($bmp)
$dc = $g.GetHdc()
$ok = [W2]::PrintWindow($h, $dc, 2)
$g.ReleaseHdc($dc); $g.Dispose()
if (-not $ok) { $bmp.Dispose(); Write-Output "ERR: PrintWindow gagal"; exit 1 }
$bmp.Save($OutFile, [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
Write-Output "OK ${w}x${t} -> $OutFile"
