package main

import (
	"flag"
	"fmt"
	"os"
)

var (
	token           string
	chatID, adminID string
	host            string
	port            int
	pass            string
	logPath         string
	statusInterval  string
)

func init() {
	flag.StringVar(&token, "token", "", "Discord bot token")
	flag.StringVar(&chatID, "chat-channel", "", "Discord chat channel ID")
	flag.StringVar(&adminID, "admin-channel", "", "Discord admin channel ID")
	flag.StringVar(&host, "host", "localhost", "Minecraft server host")
	flag.IntVar(&port, "port", 25575, "Minecraft server rcon port")
	flag.StringVar(&pass, "pass", "", "Minecraft server rcon password")
	flag.StringVar(&logPath, "log", "logs/latest.log", "Path to Minecraft server's latest.log")
	flag.StringVar(&statusInterval, "status-interval", "5m", "Interval to sync the server status with chat channel topic")

	flag.Parse()
}

func main() {
	s, err := newSyncer()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(2)
	}

	s.sync()
}
