package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mattylight22/gh-ghs/internal/output"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List profiles (nvm-style), the active one, and their pins",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, hosts, err := loadWorld()
		if err != nil {
			return err
		}
		activeProfile, _, _ := reg.ProfileForUsername(hosts.Active)

		if output.JSONMode() {
			type jsonProfile struct {
				Name     string   `json:"name"`
				Username string   `json:"username"`
				GitName  string   `json:"git_name"`
				Email    string   `json:"email"`
				Active   bool     `json:"active"`
				LoggedIn bool     `json:"logged_in"`
				Pins     []string `json:"pins,omitempty"`
			}
			logged := map[string]bool{}
			for _, u := range hosts.Users {
				logged[u] = true
			}
			var out []jsonProfile
			for _, name := range reg.ProfileNames() {
				p := reg.Profiles[name]
				jp := jsonProfile{
					Name: name, Username: p.Username, GitName: p.Name, Email: p.Email,
					Active: name == activeProfile, LoggedIn: logged[p.Username],
				}
				for _, pin := range reg.Pins {
					if pin.Profile == name {
						jp.Pins = append(jp.Pins, pin.Path)
					}
				}
				out = append(out, jp)
			}
			return output.JSON(map[string]any{"active_user": hosts.Active, "profiles": out})
		}

		if len(reg.Profiles) == 0 {
			fmt.Println("No profiles yet. Run `ghs add` to create one.")
			return nil
		}
		logged := map[string]bool{}
		for _, u := range hosts.Users {
			logged[u] = true
		}
		for _, name := range reg.ProfileNames() {
			p := reg.Profiles[name]
			marker := "   "
			label := fmt.Sprintf("%-12s %s <%s>", name, p.Username, p.Email)
			if name == activeProfile {
				marker = output.Green("-> ")
				label = output.Bold(label)
			}
			warn := ""
			if !logged[p.Username] {
				warn = " " + output.Yellow("(not logged in to gh — run `ghs add "+name+"`)")
			}
			fmt.Println(marker + label + warn)
			for _, pin := range reg.Pins {
				if pin.Profile == name {
					fmt.Println("     " + output.Dim("pin: "+pin.Path))
				}
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
