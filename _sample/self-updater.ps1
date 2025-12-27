$dir = "C:\Program Files\itrust-updater"
$src = Join-Path $dir "itrust-updater.exe"
$runner = Join-Path $dir "itrust-updater-runner.exe"

Copy-Item -Force $src $runner
& $runner get updater
Remove-Item -Force $runner -ErrorAction SilentlyContinue