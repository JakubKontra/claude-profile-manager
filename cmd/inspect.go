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
	return &cobra.Command{
		Use:   "status",
		Short: "Show sync status of all profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadConfig()
			if err != nil {
				return err
			}
			app.ShowStatus(cfg)
			return nil
		},
	}
}

func doctorCmd() *cobra.Command {
	var asJSON bool
	var opts internal.DoctorOptions
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose issues with profiles and configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := app.LoadCloudConfig()
			if err != nil {
				return err
			}
			report := app.DoctorReportFor(cfg, opts)
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
