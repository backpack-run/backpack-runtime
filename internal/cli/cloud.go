package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/backpack-run/backpack-runtime/internal/cloud"
)

func (a *app) login(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(a.err)
	name := fs.String("name", "", "device name shown in the Backpack account")
	noBrowser := fs.Bool("no-browser", false, "print the verification URL without opening it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: backpack login [--name device-name] [--no-browser]")
	}
	if strings.TrimSpace(*name) == "" {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "Backpack CLI"
		}
		*name = hostname
	}
	return a.cloud.Login(ctx, *name, func(prompt cloud.LoginPrompt) error {
		fmt.Fprintf(a.out, "Authorize Backpack Cloud in your browser.\nCode: %s\nURL:  %s\n", prompt.UserCode, prompt.VerificationURL)
		if !*noBrowser {
			if err := openBrowser(prompt.VerificationURL); err != nil {
				fmt.Fprintf(a.err, "Could not open the browser automatically: %v\n", err)
			}
		}
		fmt.Fprintln(a.out, "Waiting for approval...")
		return nil
	})
}

func (a *app) logout(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: backpack logout")
	}
	if err := a.cloud.Logout(); err != nil {
		return err
	}
	fmt.Fprintln(a.out, "Removed the local Backpack Cloud device credential.")
	if os.Getenv("BACKPACK_API_KEY") != "" {
		fmt.Fprintln(a.err, "BACKPACK_API_KEY remains set in the environment; unset it separately to end API-key authentication.")
	}
	fmt.Fprintln(a.out, "Revoke the registered device from your Backpack account to invalidate any outstanding token immediately.")
	return nil
}

func (a *app) cloudCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: backpack cloud <status|models> [--json]")
	}
	fs := flag.NewFlagSet("cloud "+args[0], flag.ContinueOnError)
	fs.SetOutput(a.err)
	asJSON := fs.Bool("json", false, "emit machine-readable output")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: backpack cloud %s [--json]", args[0])
	}
	switch args[0] {
	case "status":
		source, deviceKeyID, err := a.cloud.AuthStatus()
		if err != nil {
			return err
		}
		report := map[string]any{"endpoint": a.cloud.BaseURL, "authenticated": source != "none", "credential_source": source}
		if deviceKeyID != "" {
			report["device_key_id"] = deviceKeyID
		}
		if *asJSON {
			return writeJSON(a.out, report)
		}
		fmt.Fprintf(a.out, "Endpoint: %s\nAuthenticated: %v\nCredential: %s\n", a.cloud.BaseURL, source != "none", source)
		return nil
	case "models":
		models, err := a.cloud.Models(ctx)
		if err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(a.out, map[string]any{"object": "list", "data": models})
		}
		fmt.Fprintln(a.out, "MODEL                                      STATUS       CONTEXT    CAPABILITIES")
		for _, model := range models {
			fmt.Fprintf(a.out, "%-42s %-12s %-10d %s\n", model.ID, model.Status, model.ContextWindow, strings.Join(model.Capabilities, ","))
		}
		return nil
	default:
		return fmt.Errorf("unknown cloud command %q; use status or models", args[0])
	}
}

func writeJSON(output interface{ Write([]byte) (int, error) }, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(output, string(data))
	return err
}

func openBrowser(address string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", address)
	case "darwin":
		command = exec.Command("open", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
