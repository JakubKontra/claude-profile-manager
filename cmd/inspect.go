package cmd

import (
	"github.com/jakubkontra/cpm/internal"
	"github.com/spf13/cobra"
)

func listCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all configured profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadConfig()
			if err != nil {
				return err
			}
			entries := app.ListProfiles(cfg)
			if asJSON {
				return app.PrintJSON(entries)
			}
			app.RenderProfileList(entries)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

func statusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show sync status of all profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadConfig()
			if err != nil {
				return err
			}
			report := app.Status(cfg)
			if asJSON {
				return app.PrintJSON(report)
			}
			app.RenderStatus(report)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

func doctorCmd() *cobra.Command {
	var asJSON, fix bool
	var opts internal.DoctorOptions
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose issues with profiles and configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadCloudConfig()
			if err != nil {
				return err
			}
			app.JSON = asJSON
			var report internal.DoctorReport
			if fix {
				if report, err = app.DoctorFix(cfg, opts); err != nil {
					return err
				}
			} else {
				report = app.DoctorReportFor(cfg, opts)
			}
			if asJSON {
				if err := app.PrintJSON(report); err != nil {
					return err
				}
			} else {
				app.RenderDoctorReport(report)
			}
			if !report.OK {
				return internal.ErrDoctorFailed
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&opts.Verify, "verify", false, "read Keychain tokens to check expiry (macOS, may prompt)")
	cmd.Flags().BoolVar(&fix, "fix", false, "repair what cpm install can (missing or outdated wrappers, directories, symlinks)")
	return cmd
}

func credentialsCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "credentials",
		Short: "Show credential status for all profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadConfig()
			if err != nil {
				return err
			}
			entries := app.CredentialReport(cfg)
			if asJSON {
				return app.PrintJSON(entries)
			}
			app.RenderCredentialReport(entries)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}
