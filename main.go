package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	irc "github.com/thoj/go-ircevent"
)

var server = os.Getenv("SERVER")
var channel = os.Getenv("CHANNEL")

func main() {
	nick := "layer-d8"
	con := irc.IRC(nick, nick)
	err := con.Connect(server)
	if err != nil {
		fmt.Printf("Err %s", err)
		return
	}
	con.AddCallback("001", func(e *irc.Event) {
		var authMessage = "identify " + os.Getenv("PASS")
		con.Privmsg("NickServ", authMessage)
		// wait until authed to join channel
		time.Sleep(5 * time.Second)
		con.Join(channel)
	})
	con.AddCallback("PRIVMSG", func(e *irc.Event) {
		if strings.Contains(e.Message(), ",date") {
			currentTime := time.Now()
			con.Privmsg(channel, currentTime.Format("2006-January-02"))
		}
	})
	con.Loop()
}
