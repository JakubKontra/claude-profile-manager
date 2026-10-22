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
			cfg, base, err := loadCfg()
			if err != nil {
				return err
			}
			entries := internal.ListProfiles(cfg, base)
			if asJSON {
				return internal.PrintJSON(entries)
			}
			internal.RenderProfileList(entries)
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
			cfg, base, err := loadCfg()
			if err != nil {
				return err
			}
			internal.ShowStatus(cfg, base)
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
			cfg, err := internal.LoadCloudConfig(configPath)
			base := internal.ProfilesBaseDir(configPath)
			if err != nil {
				return err
			}
			report := internal.DoctorReportFor(cfg, base, opts)
			if asJSON {
				if err := internal.PrintJSON(report); err != nil {
					return err
				}
			} else {
				internal.RenderDoctorReport(report)
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
			cfg, base, err := loadCfg()
			if err != nil {
				return err
			}
			entries := internal.CredentialReport(cfg, base)
			if asJSON {
				return internal.PrintJSON(entries)
			}
			internal.RenderCredentialReport(entries)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}
