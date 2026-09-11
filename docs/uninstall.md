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
```

Linux or macOS:

```sh
rm "$HOME/.local/bin/backpack"
```

If you installed to a custom directory, remove `backpack` or `backpack.exe` from that exact directory instead. Remove any `PATH` entry you added manually. The installer itself does not change `PATH`.

Removing the executable preserves downloaded models, managed runtimes, outputs, configuration, and compute-target definitions so a reinstall can reuse them. Inspect the exact data root first with `backpack doctor`. Delete that directory only when you intentionally want a full reset, after backing up anything needed. Do not use a broad or unresolved recursive-delete command.

Uninstalling does not revoke SSH keys because Backpack never stores private key contents. Remove any compute-target configuration before deleting the data root if you want the configuration removed through Backpack's normal validation path.
