package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/fsnotify/fsnotify"
)

type syncer struct {
	s               *discordgo.Session
	c               *rconClient
	refreshInterval time.Duration
}

func newSyncer() (*syncer, error) {
	if token == "" {
		return nil, fmt.Errorf("Discord bot token is required")
	}
	if pass == "" {
		return nil, fmt.Errorf("Rcon password is required")
	}

	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("connecting to discord: %v", err)
	}

	s.Identify.Intents = discordgo.IntentsGuildMessages
	err = s.Open()
	if err != nil {
		return nil, fmt.Errorf("connecting to discord: %v", err)
	}

	addr := fmt.Sprintf("%v:%v", host, port)
	c := newClient(addr, pass)
	for {
		log.Println("Logging in to Minecraft rcon at", addr)
		err := c.login()
		if err == nil {
			break
		}

		log.Printf("Failed to login (%v), trying again in %v\n", err.Error(), rconRetryDelay.String())
		time.Sleep(rconRetryDelay)
	}

	var d time.Duration
	if statusInterval != "" {
		d, err = time.ParseDuration(statusInterval)
		if err != nil {
			return nil, fmt.Errorf("parse status interval: %v", err)
		}
	}

	return &syncer{s, c, d}, nil
}

func (m *syncer) forwardLogs() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating watcher: %v", err)
	}

	f, err := openFile(logPath, w)
	if err != nil {
		return fmt.Errorf("opening log: %v", err)
	}
	defer f.Close()
	// want to dump all the old logs on startup, so seek to the end
	f.Seek(0, os.SEEK_END)

	log.Println("Forwarding Minecraft logs to Discord...")
	for {
		select {
		case ev := <-w.Events:
			switch ev.Op {
			case fsnotify.Write:
				drainLogs(f, m.s)
			case fsnotify.Rename:
				log.Println("Log rotated")

				err = w.Remove(ev.Name)
				if err != nil {
					return fmt.Errorf("removing watch: %v", err)
				}

				f, err = openFile(logPath, w)
				if err != nil {
					return fmt.Errorf("reopening log: %v", err)
				}

				drainLogs(f, m.s)
			}
		case <-w.Errors:
			return fmt.Errorf("watcher: %v", err)
		}
	}
}

func (m *syncer) forwardChat() {
	m.s.AddHandler(func(s *discordgo.Session, msg *discordgo.MessageCreate) {
		if msg.ChannelID != chatID || msg.Author.ID == s.State.User.ID {
			return
		}

		payload := fmt.Sprintf("say <%v>: %v", msg.Author.Username, msg.Content)
		_, err := m.c.command(payload)
		if err != nil {
			log.Printf("Message '%v' failed to send: %v", msg.Content, err.Error())
		}
	})
	log.Println("Forwarding Discord chat to Minecraft...")
}

func (m *syncer) forwardAdmin() {
	m.s.AddHandler(func(s *discordgo.Session, msg *discordgo.MessageCreate) {
		if msg.ChannelID != adminID || msg.Author.ID == s.State.User.ID {
			return
		}

		payload := fmt.Sprintf(msg.Content)
		res, err := m.c.command(payload)
		if err != nil {
			log.Printf("Command '%v' failed to send: %v", msg.Content, err.Error())
			return
		}

		// TODO: segment lines if they exceed discord max message length of 2000
		_, err = m.s.ChannelMessageSend(msg.ChannelID, res)
		if err != nil {
			log.Printf("Message '%v' failed to send: %v", res, err.Error())
		}

	})
	log.Println("Forwarding admin Discord to Minecraft console...")
}

var listRegex = regexp.MustCompile(`There are (\d+) of a max of (\d+) players online: (.*)`)

func (m *syncer) refreshServerStatus() error {
	res, err := m.c.command("list")
	if err != nil {
		return fmt.Errorf("running `list` command: %v", err)
	}

	parts := listRegex.FindStringSubmatch(res)
	if len(parts) != 4 {
		return fmt.Errorf("unexpected result: %v", res)
	}

	topic := fmt.Sprintf("%v/%v online: %v", parts[1], parts[2], parts[3])
	_, err = m.s.ChannelEditComplex(chatID, &discordgo.ChannelEdit{Topic: topic})
	if err != nil {
		return fmt.Errorf("updating topic: %v", err)
	}
	return nil
}

func (m *syncer) syncServerStatus() {
	log.Println("Refreshing chat channel status every", m.refreshInterval.String())

	err := m.refreshServerStatus()
	if err != nil {
		log.Println("Failed to sync server status:", err.Error())
	}

	ticker := time.NewTicker(5 * time.Minute)
	for {
		select {
		case <-ticker.C:
			err := m.refreshServerStatus()
			if err != nil {
				log.Println("Failed to sync server status:", err.Error())
			}
		}
	}
}

func (m *syncer) sync() {
	if chatID != "" {
		go m.forwardChat()
	}

	if adminID != "" {
		go m.forwardAdmin()
	}

	if statusInterval != "" {
		go m.syncServerStatus()
	}

	go m.forwardLogs()

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done
}
