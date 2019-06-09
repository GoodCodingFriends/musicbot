package musicplayer

import (
	"context"
	"io"
	"os"
	"os/exec"

	"github.com/acomagu/musicbot/soundplayer"
	"github.com/djherbis/buffer"
	"github.com/djherbis/nio"
	"github.com/jonas747/dca"
)

type MusicPlayer struct {
	SoundPlayer *soundplayer.SoundPlayer
}

func (mp *MusicPlayer) Play(ctx context.Context, channelID, url string) error {
	cmd := exec.CommandContext(ctx, "youtube-dl", "-f", "bestaudio", "-o", "-", url)
	soundfile, soundfilew := nio.Pipe(buffer.New(10 * 1024 * 1024)) // 10MB
	cmd.Stdout = soundfilew
	cmd.Stderr = os.Stderr

	errC2 := make(chan error, 1)

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		errC2 <- cmd.Wait()
		soundfilew.CloseWithError(io.EOF)
	}()

	frames, errC := loadSound(soundfile)
	if err := mp.SoundPlayer.PlaySound(ctx, channelID, frames); err != nil {
		return err
	}

	// Wait for the command.
	if err := <-errC2; err != nil {
		return err
	}

	// Wait for the playing.
	if err := <-errC; err != nil {
		return err
	}

	return nil
}

func loadSound(file io.Reader) (<-chan []byte, <-chan error) {
	errC := make(chan error, 1)

	option := &dca.EncodeOptions{
		Volume:           256,
		Channels:         2,
		FrameRate:        48000,
		FrameDuration:    20,
		Bitrate:          128,
		Application:      dca.AudioApplicationAudio,
		CompressionLevel: 10,
		PacketLoss:       1,
		BufferedFrames:   524288,
		VBR:              true,
	}
	encoder, err := dca.EncodeMem(file, option)
	if err != nil {
		errC <- err
		return nil, errC
	}

	c := make(chan []byte, 1024)
	go func() {
		defer func() {
			close(c)
		}()
		for {
			frame, err := encoder.OpusFrame()
			if err == io.EOF {
				break
			}
			if err != nil {
				errC <- err
				return
			}

			c <- frame
		}
		errC <- nil
	}()

	return c, errC
}
