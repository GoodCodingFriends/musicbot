package bot

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/acomagu/musicbot/musicplayer"
	"github.com/acomagu/musicbot/soundplayer"
	"github.com/acomagu/musicbot/version"
	"github.com/pkg/errors"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var projectID = os.Getenv("GCP_PROJECT")

func init() {
	rand.Seed(time.Now().Unix())
}

type Bot struct {
	Session      Session
	SoundPlayers map[string]*soundplayer.SoundPlayer
	Joiner       soundplayer.VoiceChannelJoiner
	me           *User
}

func New(session Session, joiner soundplayer.VoiceChannelJoiner) (*Bot, error) {
	me, err := session.GetMe()
	if err != nil {
		return nil, errors.Wrap(err, "could not get own User info")
	}

	return &Bot{
		Session:      session,
		SoundPlayers: make(map[string]*soundplayer.SoundPlayer), // guildID -> SoundPlayer
		Joiner:       joiner,
		me:           me,
	}, nil
}

func (b *Bot) Wait() {
	// Wait forever.
	<-(chan struct{})(nil)
}

func (b *Bot) OnReady(event *ReadyEvent) error {
	b.Session.UpdateStatus(0, "!music")
	return nil
}

func (b *Bot) OnGuildCreate(event *GuildCreateEvent) error {
	// if event.Guild.Unavailable {
	// 	return nil
	// }
	//
	// var channel *Channel
	// for _, c := range event.Guild.Channels {
	// 	if c.IsGuildText && b.Session.HasSendMessagePermission(b.me.ID, c.ID) {
	// 		channel = c
	// 		break
	// 	}
	// }
	// if channel == nil {
	// 	return nil
	// }
	//
	// _, err := b.Session.ChannelMessageSend(channel.ID, "Commands: !add <url> [to <playlist>] | !play [<playlist>] | !add-playlist <playlist>")
	// if err != nil {
	// 	return err
	// }

	return nil
}

var urlRe = regexp.MustCompile(`[-a-zA-Z0-9@:%._\+~#=]{2,256}\.[a-z]{2,6}\b([-a-zA-Z0-9@:%_\+.~#?&//=]*)`)

type DBPlaylist struct {
	Name string
}

type DBPlaylistEntry struct {
	URL      string    `firestore:"url"`
	Inserted time.Time `firestore:"inserted"`
}

func (b *Bot) OnMessageCreate(e *MessageCreateEvent) error {
	ctx := context.Background()

	if e.Author.ID == b.me.ID {
		return nil
	}

	channelID := e.ChannelID
	guildID := e.GuildID

	switch {
	case strings.HasPrefix(e.Content, "!play"):
		//
		// Parse args
		//
		args := make([]string, 4)
		n, _ := fmt.Sscan(e.Content, &args[0], &args[1], &args[2], &args[3])
		if n > 2 {
			// TODO: send usage.
			return nil
		}
		var playlistName string
		if n == 2 {
			playlistName = args[1]
		}

		//
		// Connect to Discord
		//
		vss, err := b.Session.VoiceStates(guildID)
		if err != nil {
			return err
		}

		if _, ok := b.SoundPlayers[guildID]; !ok {
			b.SoundPlayers[guildID] = soundplayer.NewSoundPlayer(b.Joiner, guildID)
		}
		player := b.SoundPlayers[guildID]

		var voiceChannelID string
		for _, vs := range vss {
			if vs.UserID == e.Author.ID {
				voiceChannelID = vs.ChannelID
			}
		}
		if channelID == "" {
			return nil
		}

		//
		// Get Playlist
		//
		fs, err := firestore.NewClient(ctx, projectID)
		if err != nil {
			return err
		}

		playlist, err := searchPlaylist(ctx, fs, guildID, playlistName)
		if err != nil {
			return err
		}

		docs, err := playlist.OrderBy("inserted", firestore.Desc).Limit(300).Documents(ctx).GetAll()
		if err != nil {
			return err
		}

		var entries []*DBPlaylistEntry
		for _, e := range docs {
			dbPlaylistEntry := new(DBPlaylistEntry)
			if err := e.DataTo(dbPlaylistEntry); err != nil {
				return err
			}

			entries = append(entries, dbPlaylistEntry)
		}

		rand.Shuffle(len(entries), func(i, j int) {
			entries[i], entries[j] = entries[j], entries[i]
		})

		// Start playing music
		go func() {
			playErrC := make(chan error, 1)
			playErrC <- nil
			for _, entry := range entries {
				url := entry.URL
				if !strings.HasPrefix(url, "http") {
					url = fmt.Sprint("http://%s", url)
				}

				mp := musicplayer.NewMusicPlayer(player, url)

				var errC chan error
				go func() {
					time.Sleep(30) // Wait for converting the current music.
					errC <- mp.Download(ctx)
				}()

				// Wait end of the previous music.
				if err := <-playErrC; err != nil { // If fail to play the previous music, print and continue.
					fmt.Fprintln(os.Stderr, err)
				}

				hasUser, err := b.hasUserExceptForMe(guildID, voiceChannelID)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					break
				}
				if !hasUser {
					break
				}

				if _, err := b.Session.ChannelMessageSend(channelID, fmt.Sprintf(":musical_note: Now Playing :musical_note:\n%s", url)); err != nil {
					fmt.Fprintln(os.Stderr, err)
					break
				}

				go func() {
					playErrC <- mp.Play(ctx, voiceChannelID)
				}()
			}

			// TODO: Leave from the voice channel.
		}()

	case strings.HasPrefix(e.Content, "!add "):
		//
		// Interpret the args
		//
		args := make([]string, 4)
		n, _ := fmt.Sscan(e.Content, &args[0], &args[1], &args[2], &args[3])
		if n < 2 || n > 3 {
			// TODO: send usage.
			return nil
		}
		var url, playlistName string
		for _, s := range args[1:n] {
			switch {
			case urlRe.MatchString(s):
				if url != "" {
					// TODO: send usage.
					return nil
				}
				url = s
			case s == "to":
			default:
				if playlistName != "" {
					// TODO: send usage.
					return nil
				}
				playlistName = s
			}
		}

		if url == "" {
			// TODO: send usage.
			return nil
		}

		//
		// Determine the playlist to add.
		//
		fs, err := firestore.NewClient(ctx, projectID)
		if err != nil {
			return err
		}

		playlist, err := searchPlaylist(ctx, fs, guildID, playlistName)
		if err != nil {
			return err
		}

		//
		// Add
		//
		if _, _, err := playlist.Add(ctx, &DBPlaylistEntry{
			URL:      url,
			Inserted: time.Now(),
		}); err != nil {
			return err
		}

		// TODO: Feedback.

	case strings.HasPrefix(e.Content, "!add-playlist "):
		//
		// Parse args
		//
		args := make([]string, 4)
		n, _ := fmt.Sscan(e.Content, &args[0], &args[1], &args[2], &args[3])
		if n != 2 {
			// TODO: send usage.
			return nil
		}
		playlistName := args[1]

		//
		// Create playlist
		//
		fs, err := firestore.NewClient(ctx, projectID)
		if err != nil {
			return err
		}

		// Create guild doc if not exist.
		if _, err := fs.Collection("guilds").Doc(guildID).Get(ctx); grpc.Code(err) == codes.NotFound {
			if _, err := fs.Collection("guilds").Doc(guildID).Create(ctx, struct{}{}); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}

		if _, _, err := fs.Collection("guilds").Doc(guildID).Collection("playlists").Add(ctx, &DBPlaylist{
			Name: playlistName,
		}); err != nil {
			return err
		}

		// Feedback

	case strings.HasPrefix(e.Content, "!version"):
		if _, err := b.Session.ChannelMessageSend(channelID, version.Version); err != nil {
			return err
		}
	}

	return nil
}

func (b *Bot) nextMusic() (string, bool) {
	return "https://youtu.be/R2qfiygqjh8", true // TODO
}

func (b *Bot) hasUserExceptForMe(guildID, voiceChannelID string) (bool, error) {
	vss, err := b.Session.VoiceStates(guildID)
	if err != nil {
		return false, err
	}

	for _, vs := range vss {
		if vs.ChannelID == voiceChannelID && vs.UserID != b.me.ID {
			return true, nil
		}
	}
	return false, nil
}

func searchPlaylist(ctx context.Context, fs *firestore.Client, guildID, playlistName string) (*firestore.CollectionRef, error) {
	playlists := fs.Collection("guilds").Doc(guildID).Collection("playlists")

	var playlist *firestore.CollectionRef
	iter := playlists.Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		dbPlaylist := new(DBPlaylist)
		if err := doc.DataTo(dbPlaylist); err != nil {
			return nil, err
		}

		if strings.Contains(dbPlaylist.Name, playlistName) {
			playlist = doc.Ref.Collection("playlist")
			break
		}
	}
	if playlist == nil {
		return nil, fmt.Errorf("no playlist") // TODO
	}

	return playlist, nil
}

// var diceregexp = regexp.MustCompile(`(\d+)[dD](\d+)`)

// var help = `?? よくわかりません :sweat_drops: "!dice 2D10"などとタイプしてみてください :bow:`

// func calc(msg string) (string, bool) {
// 	groups := diceregexp.FindStringSubmatch(msg)
// 	if groups == nil || len(groups) < 3 {
// 		return "", false
// 	}
//
// 	n, err := strconv.Atoi(groups[1])
// 	if err != nil {
// 		return "", false
// 	}
//
// 	h, err := strconv.Atoi(groups[2])
// 	if err != nil {
// 		return "", false
// 	}
//
// 	ans := 0
// 	for i := 0; i < n; i++ {
// 		ans += rand.Intn(h) + 1
// 	}
// 	return fmt.Sprint(ans), true
// }
