package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/virat-mankali/discord-cli/internal/config"
	"github.com/virat-mankali/discord-cli/internal/discord"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show the currently authenticated Discord user",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := resolvedStore()
		token, err := discord.LoadToken(config.TokenPath(store))
		if err != nil {
			return err
		}

		session, err := discord.NewSession(token)
		if err != nil {
			return err
		}
		defer session.Close()

		me, err := session.User("@me")
		if err != nil {
			return fmt.Errorf("failed to fetch user info: %w", err)
		}

		fmt.Printf("Logged in as %s#%s\n", me.Username, me.Discriminator)
		fmt.Printf("  ID:    %s\n", me.ID)
		fmt.Printf("  Email: %s\n", me.Email)
		fmt.Printf("  Token: %s\n", token.TokenType)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}
