package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mattylight22/gh-ghs/internal/ghcli"
	"github.com/mattylight22/gh-ghs/internal/gitcfg"
	"github.com/mattylight22/gh-ghs/internal/output"
	"github.com/mattylight22/gh-ghs/internal/registry"
)

var (
	flagAddUsername string
	flagAddName     string
	flagAddEmail    string
)

var addCmd = &cobra.Command{
	Use:   "add [<profile>]",
	Short: "Create a profile (and log the account in to gh if needed)",
	Long: "Creates a named profile mapping a gh account to a git identity.\n" +
		"Interactive on a TTY; non-interactive callers must pass --username, --name and --email.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, hosts, err := loadWorld()
		if err != nil {
			return err
		}

		name := ""
		if len(args) == 1 {
			name = args[0]
		}
		if name == "" {
			name = output.Prompt("Profile name (e.g. personal, work)", "")
		}
		if name == "" {
			return output.Usagef("profile name required (usage: ghs add <profile> [--username --name --email])")
		}

		username := flagAddUsername
		if username == "" {
			def := ""
			if existing, ok := reg.Profiles[name]; ok {
				def = existing.Username
			}
			username = output.Prompt("GitHub username for this profile", def)
		}
		if username == "" {
			return output.Usagef("--username required in non-interactive mode")
		}

		gitName := flagAddName
		if gitName == "" {
			gitName = output.Prompt("Git author name (user.name)", username)
		}
		if gitName == "" {
			return output.Usagef("--name required in non-interactive mode")
		}

		email := flagAddEmail
		if email == "" {
			def := ""
			if existing, ok := reg.Profiles[name]; ok {
				def = existing.Email
			}
			email = output.Prompt("Git author email (user.email)", def)
		}
		if email == "" {
			return output.Usagef("--email required in non-interactive mode")
		}

		// Log the account in to gh if it isn't yet.
		loggedIn := false
		for _, u := range hosts.Users {
			if u == username {
				loggedIn = true
			}
		}
		if !loggedIn {
			if err := ghcli.EnsureUsable(); err != nil {
				return output.Envf("%v", err)
			}
			if !output.Interactive() {
				return output.Envf("account %q is not logged in to gh; run `gh auth login` (interactive) first", username)
			}
			fmt.Printf("Account %s is not logged in to gh yet — starting `gh auth login`.\n", output.Bold(username))
			fmt.Println(output.Yellow("Make sure you authenticate as " + username + "."))
			if err := ghcli.AuthLogin(); err != nil {
				return output.Envf("gh auth login failed: %v", err)
			}
			after, err := ghcli.ReadHosts()
			if err == nil {
				found := false
				for _, u := range after.Users {
					if u == username {
						found = true
					}
				}
				if !found {
					fmt.Println(output.Yellow("! gh login completed but account " + username + " was not among the logged-in users; check `gh auth status`."))
				}
			}
		}

		reg.Profiles[name] = registry.Profile{Username: username, Name: gitName, Email: email}

		// If this profile matches the currently active gh user, sync identity now.
		hostsNow, _ := ghcli.ReadHosts()
		if hostsNow.Active == username {
			if err := gitcfg.EnsureSetup(reg); err != nil {
				return output.Envf("%v", err)
			}
			if err := gitcfg.Regenerate(reg, name); err != nil {
				return output.Envf("%v", err)
			}
		}
		if err := reg.Save(); err != nil {
			return output.Envf("%v", err)
		}

		if output.JSONMode() {
			return output.JSON(map[string]any{"profile": name, "username": username, "email": email})
		}
		fmt.Printf("Profile %s saved (%s <%s>).\n", output.Bold(name), username, email)
		fmt.Println("Switch with `ghs use " + name + "`, or pin a folder with `ghs pin " + name + " <dir>`.")
		return nil
	},
}

func init() {
	addCmd.Flags().StringVar(&flagAddUsername, "username", "", "GitHub account login")
	addCmd.Flags().StringVar(&flagAddName, "name", "", "git author name")
	addCmd.Flags().StringVar(&flagAddEmail, "email", "", "git author email")
	rootCmd.AddCommand(addCmd)
}
