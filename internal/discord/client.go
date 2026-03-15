package discord

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
)

// NewSession creates an authenticated discordgo session.
func NewSession(token *StoredToken) (*discordgo.Session, error) {
	s, err := discordgo.New(token.FormatToken())
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}
	// Validate token
	_, err = s.User("@me")
	if err != nil {
		return nil, fmt.Errorf("invalid token — run `discocli auth` again: %w", err)
	}
	return s, nil
}

// FetchMessages paginates through messages in a channel oldest-first.
// Pass beforeID to start from a specific point (for resumable sync).
// Calls onBatch for each batch of up to 100 messages.
func FetchMessages(s *discordgo.Session, channelID string, onBatch func([]*discordgo.Message) error, beforeID string) error {
	for {
		msgs, err := s.ChannelMessages(channelID, 100, beforeID, "", "")
		if err != nil {
			return fmt.Errorf("failed to fetch messages: %w", err)
		}
		if len(msgs) == 0 {
			break
		}
		if err := onBatch(msgs); err != nil {
			return err
		}
		beforeID = msgs[len(msgs)-1].ID
		time.Sleep(500 * time.Millisecond) // respect rate limits
	}
	return nil
}

// ResolveChannelByName finds a text channel by name within a guild.
func ResolveChannelByName(s *discordgo.Session, guildID, name string) (*discordgo.Channel, error) {
	channels, err := s.GuildChannels(guildID)
	if err != nil {
		return nil, fmt.Errorf("failed to list channels: %w", err)
	}
	for _, ch := range channels {
		if ch.Name == name && ch.Type == discordgo.ChannelTypeGuildText {
			return ch, nil
		}
	}
	return nil, fmt.Errorf("channel %q not found in guild %s", name, guildID)
}

// ResolveGuildByName finds a guild by name from the user's guild list.
func ResolveGuildByName(s *discordgo.Session, name string) (*discordgo.UserGuild, error) {
	guilds, err := s.UserGuilds(100, "", "", false)
	if err != nil {
		return nil, fmt.Errorf("failed to list guilds: %w", err)
	}
	for _, g := range guilds {
		if g.Name == name {
			return g, nil
		}
	}
	return nil, fmt.Errorf("guild %q not found", name)
}
