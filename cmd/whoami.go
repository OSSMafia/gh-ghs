package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/OSSMafia/gh-ghs/internal/output"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Print the active gh account",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, hosts, err := loadWorld()
		if err != nil {
			return err
		}
		profile, _, _ := reg.ProfileForUsername(hosts.Active)
		if output.JSONMode() {
			return output.JSON(map[string]string{"active_user": hosts.Active, "active_profile": profile})
		}
		if hosts.Active == "" {
			return output.Envf("no gh account logged in — run `ghs add`")
		}
		if profile != "" {
			fmt.Printf("%s (profile: %s)\n", hosts.Active, profile)
		} else {
			fmt.Println(hosts.Active)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}
