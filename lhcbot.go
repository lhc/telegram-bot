//        DO WHAT THE FUCK YOU WANT TO PUBLIC LICENSE
//                     Version 2, December 2004
//
//  Copyright (C) 2016 Leandro Pereira <leandro@tia.mat.br>
//
//  Everyone is permitted to copy and distribute verbatim or modified
//  copies of this license document, and changing it is allowed as long
//  as the name is changed.
//
//             DO WHAT THE FUCK YOU WANT TO PUBLIC LICENSE
//    TERMS AND CONDITIONS FOR COPYING, DISTRIBUTION AND MODIFICATION
//
//   0. You just DO WHAT THE FUCK YOU WANT TO.

package main

import (
	"encoding/json"
	"fmt"
	"github.com/tucnak/telebot"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Status struct {
	Open       bool  `json:"open"`
	LastChange int64 `json:"lastchange"`
}

func fetchStatus(status chan Status) {
	old_status := Status{false, 0}
	for {
		resp, err := http.Get("https://lhc.net.br/spacenet.json")
		if err != nil {
			log.Printf("Could not get status JSON (%s), waiting 5 minutes.", err)
			time.Sleep(5 * time.Second)
			continue
		}
		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Could not read JSON response: %s", err)
			continue
		}

		new_status := Status{false, 0}
		err = json.Unmarshal([]byte(body), &new_status)
		if err != nil {
			log.Printf("Could not unmarshal JSON response: %s", err)
			continue
		}

		if new_status.Open != old_status.Open || old_status.LastChange == 0 {
			old_status = new_status
			status <- new_status
		}

		time.Sleep(5 * time.Minute)
	}
}

func main() {
	bot, err := telebot.NewBot(os.Getenv("BOT_ID"))
	if err != nil {
		return
	}

	messages := make(chan telebot.Message)
	bot.Listen(messages, 1*time.Second)

	status := make(chan Status)
	go fetchStatus(status)

	estado := "desconhecido"
	desde := "desconhecido"
	ultimoEstado := "Nao sei qual o estado do LHC."

	saoPaulo, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("Could not load location for timezone purposes")
	}

	dest := telebot.Chat{-48083180, "group", "LHC", "", "", ""}

	for {
		select {
		case s := <-status:
			if s.Open {
				estado = "aberto"
			} else {
				estado = "fechado"
			}
			tm := time.Unix(s.LastChange, 0).In(saoPaulo)
			desde = tm.Format(time.ANSIC)

			fmt.Printf("Mudança de estado: %s em %s\n", estado, desde)

			ultimoEstado = "O LHC está " + estado + " desde " + desde + "."
			bot.SendMessage(dest, ultimoEstado, nil)

		case message := <-messages:
			fmt.Printf("Recebi mensagem: %s de: %s (%v)\n", message.Text,
				message.Chat, message.Chat)
			if strings.HasPrefix(message.Text, "/status") {
				bot.SendMessage(message.Chat, ultimoEstado, nil)
			}
		}
	}
}
