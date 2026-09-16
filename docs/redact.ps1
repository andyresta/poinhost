# Mengganti teks yang bisa mengidentifikasi infrastruktur di screenshot README.
#
# Cara kerjanya: sampel warna latar dari satu titik yang dipastikan kosong,
# tutup area teks dengan warna itu, lalu tulis ulang teks pengganti memakai
# font yang sama. Hasilnya menyatu dengan UI, tidak seperti kotak hitam.
#
# Spec dibaca dari file, satu region per baris, dipisah "|":
#   x|y|w|h|sampleX|sampleY|fontPx|bold|teks pengganti
# teks kosong = area hanya ditutup, tanpa tulisan pengganti.
param(
    [Parameter(Mandatory = $true)][string]$In,
    [Parameter(Mandatory = $true)][string]$Out,
    [Parameter(Mandatory = $true)][string]$Spec
)

Add-Type -AssemblyName System.Drawing

$src = [System.Drawing.Image]::FromFile((Resolve-Path $In))
$bmp = New-Object System.Drawing.Bitmap $src
$src.Dispose()

$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.TextRenderingHint = [System.Drawing.Text.TextRenderingHint]::ClearTypeGridFit
$g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias

# Warna teks UI poinhost pada tema gelap.
$fg = [System.Drawing.Color]::FromArgb(236, 229, 216)

$n = 0
foreach ($line in (Get-Content $Spec)) {
    $line = $line.Trim()
    if ($line -eq "" -or $line.StartsWith("#")) { continue }
    $p = $line.Split("|")
    if ($p.Count -lt 8) { continue }

    $x = [int]$p[0]; $y = [int]$p[1]; $w = [int]$p[2]; $h = [int]$p[3]
    $sx = [int]$p[4]; $sy = [int]$p[5]; $fs = [single]$p[6]
    $bold = ($p[7] -eq "1")
    $txt = if ($p.Count -ge 9) { $p[8] } else { "" }

    $bg = $bmp.GetPixel($sx, $sy)
    $brush = New-Object System.Drawing.SolidBrush $bg
    $g.FillRectangle($brush, $x, $y, $w, $h)
    $brush.Dispose()

    if ($txt -ne "") {
        $style = if ($bold) { [System.Drawing.FontStyle]::Bold } else { [System.Drawing.FontStyle]::Regular }
        $font = New-Object System.Drawing.Font("Segoe UI", $fs, $style, [System.Drawing.GraphicsUnit]::Pixel)
        $tb = New-Object System.Drawing.SolidBrush $fg
        # Teks diratakan kiri-tengah terhadap kotaknya supaya sejajar dengan
        # baris tabel di sekitarnya.
        $sf = New-Object System.Drawing.StringFormat
        $sf.LineAlignment = [System.Drawing.StringAlignment]::Center
        $rect = New-Object System.Drawing.RectangleF $x, $y, $w, $h
        $g.DrawString($txt, $font, $tb, $rect, $sf)
        $font.Dispose(); $tb.Dispose(); $sf.Dispose()
    }
    $n++
}

$g.Dispose()
$bmp.Save((Join-Path (Split-Path -Parent (Resolve-Path $In)) (Split-Path -Leaf $Out)), [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
Write-Output "OK $n region -> $Out"
