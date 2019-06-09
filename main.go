package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/acomagu/musicbot/bot"
	"github.com/acomagu/musicbot/discord"
	"github.com/acomagu/musicbot/router"
	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
)

var token = os.Getenv("DISCORD_TOKEN")
var port = os.Getenv("PORT")

func main() {
	if port == "" {
		port = "80"
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	go func() {
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if token == "" {
		return errors.New("No token provided. Set DISCORD_TOKEN.")
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return err
	}

	joiner := discord.NewVoiceChannelJoiner(session)

	r := &router.Router{
		Session: session,
	}

	bot, err := bot.New(discord.NewSession(session), joiner)
	if err != nil {
		return err
	}

	r.Handle(bot)

	if err := session.Open(); err != nil {
		return err
	}
	defer session.Close()
	bot.Wait()

	return nil
}
