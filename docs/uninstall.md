# Uninstall Backpack Runtime

Stop model sessions before uninstalling:

```text
backpack ps
backpack stop <session-id>
```

Close terminals or services still running `backpack`, then remove only the installed executable.

Windows PowerShell:

```powershell
Remove-Item -LiteralPath "$env:LOCALAPPDATA\Programs\Backpack\bin\backpack.exe"
$installDirectory = "$env:LOCALAPPDATA\Programs\Backpack\bin"
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$updatedPath = (($userPath -split ';') | Where-Object { $_ -and $_.TrimEnd('\') -ine $installDirectory.TrimEnd('\') }) -join ';'
[Environment]::SetEnvironmentVariable('Path', $updatedPath, 'User')
```

Linux or macOS:

```sh
rm "$HOME/.local/bin/backpack"
```

The POSIX installer labels the line it adds with `# Added by Backpack Runtime installer`; remove that line and the immediately following `case` or `fish_add_path` line from `~/.profile`, `~/.zprofile`, or `~/.config/fish/config.fish`. If you installed to a custom directory, remove `backpack` or `backpack.exe` from that exact directory and remove any custom `PATH` entry you added manually.

Removing the executable preserves downloaded models, managed runtimes, outputs, configuration, and compute-target definitions so a reinstall can reuse them. Inspect the exact data root first with `backpack doctor`. Delete that directory only when you intentionally want a full reset, after backing up anything needed. Do not use a broad or unresolved recursive-delete command.

Uninstalling does not revoke SSH keys because Backpack never stores private key contents. Remove any compute-target configuration before deleting the data root if you want the configuration removed through Backpack's normal validation path.
