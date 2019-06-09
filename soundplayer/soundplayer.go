package soundplayer

import (
	"context"
	"fmt"
	"os"
	"time"
)

type VC interface {
	Close() error
	Write(context.Context, <-chan []byte) error
	Speaking(bool) error
}

type VoiceChannelJoiner interface {
	JoinVoiceChannel(string, string) (VC, error)
}

type SoundPlayer struct {
	guildID   string
	soundChan chan<- playSoundArgs
	sp        *soundPlayer
}

func NewSoundPlayer(joiner VoiceChannelJoiner, guildID string) *SoundPlayer {
	sp := &SoundPlayer{
		sp: &soundPlayer{
			GuildID:      guildID,
			Joiner:       joiner,
			idletime:     10 * time.Second,
			speakbuftime: 250 * time.Millisecond,
		},
		guildID: guildID,
	}

	sp.soundChan = sp.sp.Start()

	return sp
}

func (sp *SoundPlayer) setIdleTime(d time.Duration) {
	sp.sp.idletime = d
}

func (sp *SoundPlayer) setSpeakBufTime(d time.Duration) {
	sp.sp.speakbuftime = d
}

func (sp *SoundPlayer) PlaySound(ctx context.Context, channelID string, frames <-chan []byte) error {
	errCh := make(chan error)
	sp.soundChan <- playSoundArgs{
		ctx:       ctx,
		ChannelID: channelID,
		frames:    frames,
		errCh:     errCh,
	}

	return <-errCh
}

type playSoundArgs struct {
	ctx       context.Context
	ChannelID string
	frames    <-chan []byte
	errCh     chan<- error
}

type soundPlayer struct {
	GuildID      string
	Joiner       VoiceChannelJoiner
	idletime     time.Duration
	speakbuftime time.Duration
}

func (sp *soundPlayer) Start() chan<- playSoundArgs {
	soundChan := make(chan playSoundArgs)
	go func(soundChan <-chan playSoundArgs) {
	RETRY:
		for args := range soundChan {
			fmt.Println("ABC")
			vc, err := sp.Joiner.JoinVoiceChannel(sp.GuildID, args.ChannelID)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			fmt.Println("D")

			currentChannelID := args.ChannelID

			args.errCh <- sp.playSound(args.ctx, vc, args.frames)
			close(args.errCh)

			timer := time.NewTimer(sp.idletime)

		L:
			for {
				select {
				case args, ok := <-soundChan:
					if !ok {
						break L
					}
					if args.ChannelID != currentChannelID {
						var err error
						vc, err = sp.changeChannel(vc, args.ChannelID)
						if err != nil {
							args.errCh <- err
							close(args.errCh)
							break RETRY
						}
						currentChannelID = args.ChannelID
					}

					timer.Reset(sp.idletime)
					args.errCh <- sp.playSound(args.ctx, vc, args.frames)
					close(args.errCh)
				case <-timer.C:
					break L
				}
			}

			// Disconnect
			vc.Close()
		}
	}(soundChan)

	return soundChan
}

func (sp *soundPlayer) changeChannel(vc VC, channelID string) (VC, error) {
	vc.Close()

	return sp.Joiner.JoinVoiceChannel(sp.GuildID, channelID)
}

func (sp *soundPlayer) playSound(ctx context.Context, vc VC, frames <-chan []byte) error {
	time.Sleep(sp.speakbuftime)

	fmt.Println("F")

	vc.Speaking(true)
	defer func() {
		time.Sleep(sp.speakbuftime)
		vc.Speaking(false)
	}()

	fmt.Println("PLAYING")
	return vc.Write(ctx, frames)
}
