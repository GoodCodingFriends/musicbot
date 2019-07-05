package musicplayer

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/acomagu/musicbot/soundplayer"
	"github.com/jonas747/dca"
)

var bufPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

type MusicPlayer struct {
	sp       *soundplayer.SoundPlayer
	url      string
	buf      *bytes.Buffer
	download sync.Once
}

func NewMusicPlayer(sp *soundplayer.SoundPlayer, url string) *MusicPlayer {
	return &MusicPlayer{
		sp:  sp,
		url: url,
		buf: bufPool.Get().(*bytes.Buffer),
	}
}

func (mp *MusicPlayer) Download(ctx context.Context) error {
	var er error
	mp.download.Do(func() {
		cmd := exec.CommandContext(ctx, "youtube-dl", "--no-playlist", "-f", "bestaudio[asr<=50000][abr<=200]/bestaudio/worst[asr>=40000][abr>=120]/worst[abr>=90]/best", "-o", "-", mp.url)

		cmd.Stdout = mp.buf
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			er = err
		}

		er = cmd.Wait()
	})

	return er
}

// Don't run concurrently with Download().
func (mp *MusicPlayer) Play(ctx context.Context, channelID string) error {
	if err := mp.Download(ctx); err != nil { // Ensure
		return err
	}

	frames, err := loadSound(mp.buf)
	if err != nil {
		return err
	}

	mp.buf.Reset()
	bufPool.Put(mp.buf)

	// No concurrent for low performance env.
	frameC := make(chan []byte, len(frames))
	for _, frame := range frames {
		frameC <- frame
	}
	close(frameC)

	if err := mp.sp.PlaySound(ctx, channelID, frameC); err != nil {
		return err
	}

	return nil
}

func loadSound(file io.Reader) ([][]byte, error) {
	option := &dca.EncodeOptions{
		Volume:           256,
		Channels:         2,
		FrameRate:        48000,
		FrameDuration:    40,
		Bitrate:          128,
		Application:      dca.AudioApplicationAudio,
		CompressionLevel: 10,
		PacketLoss:       1,
		BufferedFrames:   1024,
		VBR:              true,
	}
	encoder, err := dca.EncodeMem(file, option)
	if err != nil {
		return nil, err
	}

	var frames [][]byte
	for {
		frame, err := encoder.OpusFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		frames = append(frames, frame)
	}

	return frames, nil
}
