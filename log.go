package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/fsnotify/fsnotify"
)

const (
	logRetryDelay = 500 * time.Millisecond
)

func openFile(path string, watcher *fsnotify.Watcher) (f *os.File, err error) {
	for {
		f, err = os.Open(path)

		if err == nil {
			err = watcher.Add(path)
			if err != nil {
				return nil, fmt.Errorf("adding watch: %v", err)
			}

			return
		}

		log.Printf("Failed to open %v, trying again in %v\n", path, logRetryDelay.String())
		time.Sleep(logRetryDelay)
	}
}

var logSuffix = regexp.MustCompile(`\[.*\]+?: (.*)`)

// trim the '[00:00:00][Server thread/INFO]:' prefix
func trimPrefix(msg string) string {
	matches := logSuffix.FindStringSubmatch(msg)
	if len(matches) != 2 {
		log.Println("Weird log line:", msg)
	}
	return matches[1]
}

var (
	infoRegex = regexp.MustCompile(`\[Server thread\/INFO\]`)
	filters   = []*regexp.Regexp{
		regexp.MustCompile(`Can't keep up! Is the server overloaded?`),
		regexp.MustCompile(`\(vehicle of .+\) moved too quickly!`),
		regexp.MustCompile(`Thread RCON Client`),
		regexp.MustCompile(`logged in with entity id \d+ at \(.*\)`),
	}
)

// filter out some of the noisier logs
func filterLogs(msg string) bool {
	if !infoRegex.MatchString(msg) {
		return true
	}

	for _, re := range filters {
		if re.MatchString(msg) {
			return true
		}
	}
	return false
}

func drainLogs(file *os.File, s *discordgo.Session) error {
	f := bufio.NewScanner(file)
	for f.Scan() {
		msg := f.Text()

		if adminID != "" {
			_, err := s.ChannelMessageSend(adminID, msg)
			if err != nil {
				return fmt.Errorf("sending admin channel: %v", err)
			}
		}

		if chatID != "" && !filterLogs(msg) {
			msg = trimPrefix(msg)
			_, err := s.ChannelMessageSend(chatID, msg)
			if err != nil {
				return fmt.Errorf("sending chat channel: %v", err)
			}
		}
	}
	return nil
}
