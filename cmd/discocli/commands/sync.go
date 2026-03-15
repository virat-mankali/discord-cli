package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/virat-mankali/discord-cli/internal/config"
	"github.com/virat-mankali/discord-cli/internal/discord"
	"github.com/virat-mankali/discord-cli/internal/storage"
)

var (
	syncGuild   string
	syncChannel string
	syncFollow  bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync Discord messages to local database",
	Long: `Sync message history from Discord into the local SQLite database.
Without flags, syncs all accessible channels.
Use --follow to keep running and capture new messages in real-time.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store := resolvedStore()
		if err := config.EnsureStore(store); err != nil {
			return err
		}

		token, err := discord.LoadToken(config.TokenPath(store))
		if err != nil {
			return err
		}
		session, err := discord.NewSession(token)
		if err != nil {
			return err
		}
		defer session.Close()

		db, err := storage.Open(config.DBPath(store))
		if err != nil {
			return err
		}
		defer db.Close()

		if syncFollow {
			return runGatewaySync(session, db)
		}

		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Syncing messages..."
		s.Start()

		count, err := performHistoricalSync(session, db)
		s.Stop()

		if err != nil {
			return err
		}

		stats, _ := db.Stats()
		fmt.Printf("✅ Synced %d new messages\n", count)
		fmt.Printf("   Database: %d guilds, %d channels, %d total messages\n",
			stats.Guilds, stats.Channels, stats.Messages)
		return nil
	},
}

func performHistoricalSync(session *discordgo.Session, db *storage.DB) (int, error) {
	// Resolve which channels to sync
	channelIDs, err := resolveTargetChannels(session, db)
	if err != nil {
		return 0, err
	}

	totalCount := 0
	for _, chID := range channelIDs {
		count, err := syncChannelHistory(session, db, chID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to sync channel %s: %v\n", chID, err)
			continue
		}
		totalCount += count
	}
	return totalCount, nil
}

func resolveTargetChannels(session *discordgo.Session, db *storage.DB) ([]string, error) {
	// If specific channel ID given
	if syncChannel != "" && isSnowflake(syncChannel) {
		return []string{syncChannel}, nil
	}

	// Get guilds to scan
	var guildIDs []string
	if syncGuild != "" {
		if isSnowflake(syncGuild) {
			guildIDs = []string{syncGuild}
		} else {
			g, err := discord.ResolveGuildByName(session, syncGuild)
			if err != nil {
				return nil, err
			}
			guildIDs = []string{g.ID}
		}
	} else {
		guilds, err := session.UserGuilds(100, "", "", false)
		if err != nil {
			return nil, fmt.Errorf("failed to list guilds: %w", err)
		}
		for _, g := range guilds {
			guildIDs = append(guildIDs, g.ID)
		}
	}

	var channelIDs []string
	for _, gID := range guildIDs {
		// Store guild metadata
		guild, err := session.Guild(gID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to fetch guild %s: %v\n", gID, err)
			continue
		}
		_ = db.UpsertGuild(guild.ID, guild.Name, guild.Icon)

		channels, err := session.GuildChannels(gID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to list channels for guild %s: %v\n", gID, err)
			continue
		}
		for _, ch := range channels {
			if ch.Type != discordgo.ChannelTypeGuildText {
				continue
			}
			// Filter by channel name if specified
			if syncChannel != "" && ch.Name != syncChannel {
				continue
			}
			_ = db.UpsertChannel(ch.ID, gID, ch.Name, int(ch.Type), ch.Topic)
			channelIDs = append(channelIDs, ch.ID)
		}
	}
	return channelIDs, nil
}

func syncChannelHistory(session *discordgo.Session, db *storage.DB, channelID string) (int, error) {
	// Load existing sync state for incremental sync
	state, err := db.GetSyncState(channelID)
	if err != nil {
		return 0, err
	}

	count := 0
	var newestID, oldestID string

	// Phase A: fetch NEW messages since last sync (afterID pagination)
	if state != nil && state.LastMessageID != "" {
		afterID := state.LastMessageID
		for {
			msgs, err := session.ChannelMessages(channelID, 100, "", afterID, "")
			if err != nil || len(msgs) == 0 {
				break
			}
			for _, m := range msgs {
				if err := upsertMsg(db, m); err != nil {
					return count, err
				}
				if newestID == "" || m.ID > newestID {
					newestID = m.ID
				}
				count++
			}
			afterID = msgs[0].ID // Discord returns newest-first; walk forward
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Phase B: fetch HISTORICAL messages (beforeID pagination, resume from oldest)
	beforeID := ""
	if state != nil && state.OldestMessageID != "" {
		beforeID = state.OldestMessageID // resume from where we stopped
	}

	err = discord.FetchMessages(session, channelID, func(msgs []*discordgo.Message) error {
		for _, m := range msgs {
			if err := upsertMsg(db, m); err != nil {
				return err
			}
			if newestID == "" || m.ID > newestID {
				newestID = m.ID
			}
			if oldestID == "" || m.ID < oldestID {
				oldestID = m.ID
			}
			count++
		}
		return nil
	}, beforeID)
	if err != nil {
		return count, err
	}

	// Persist sync state
	if newestID != "" || oldestID != "" {
		n := newestID
		o := oldestID
		if state != nil {
			if state.LastMessageID > n {
				n = state.LastMessageID
			}
			if o == "" || (state.OldestMessageID != "" && state.OldestMessageID < o) {
				o = state.OldestMessageID
			}
		}
		_ = db.UpdateSyncState(channelID, n, o)
	}
	return count, nil
}

func upsertMsg(db *storage.DB, m *discordgo.Message) error {
	return db.UpsertMessage(
		m.ID, m.ChannelID, m.GuildID,
		m.Author.ID, m.Author.Username,
		m.Content, m.Timestamp,
		m.EditedTimestamp != nil && !m.EditedTimestamp.IsZero(),
	)
}

func runGatewaySync(session *discordgo.Session, db *storage.DB) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		ts := m.Timestamp
		_ = db.UpsertMessage(
			m.ID, m.ChannelID, m.GuildID,
			m.Author.ID, m.Author.Username,
			m.Content, ts, false,
		)
		name, _ := db.GetChannelName(m.ChannelID)
		if name == "" {
			name = m.ChannelID
		}
		fmt.Printf("[%s] #%s — %s: %s\n",
			ts.Format("15:04:05"), name, m.Author.Username, m.Content)
	})

	session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages
	if err := session.Open(); err != nil {
		return fmt.Errorf("failed to open gateway: %w", err)
	}
	defer session.Close()

	fmt.Println("🔴 Live sync active — press Ctrl+C to stop")
	<-ctx.Done()
	fmt.Println("\nStopping...")
	return nil
}

func init() {
	syncCmd.Flags().StringVar(&syncGuild, "guild", "", "Guild (server) name or ID to sync")
	syncCmd.Flags().StringVar(&syncChannel, "channel", "", "Channel name or ID to sync")
	syncCmd.Flags().BoolVar(&syncFollow, "follow", false, "Keep running and sync new messages in real-time")
	rootCmd.AddCommand(syncCmd)
}
