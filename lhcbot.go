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
	"github.com/PuloV/ics-golang"
	"github.com/tucnak/telebot"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ThingSpeakAPIKey  string `json:"thingspeak_api_key"`
	ThingSpeakChannel int    `json:"thingspeak_channel_id"`

	TelegramAPIKey string `json:"telegram_api_key"`
	GroupIdStr     string `json:"telegram_group_id"`
	GroupType      string `json:"telegram_group_type"`
	GroupName      string `json:"telegram_group_name"`

	GroupId int64
}

type Status struct {
	Open       bool  `json:"open"`
	LastChange int64 `json:"lastchange"`
}

type Whois struct {
	Who         []string `json:"who"`
	UnknownMacs int      `json:"n_unknown_macs"`
}

type Financas struct {
	ActualExpenses          string `json:"actual_expenses"`
	ActualIncomes           string `json:"actual_incomes"`
	RegularExpensesEstimate string `json:"regular_expenses_estimate"`
}

var config Config

func loadConfig() Config {
	home, gotHome := os.LookupEnv("HOME")
	if !gotHome {
		log.Fatal("Could not get HOME environment variable")
	}

	name := fmt.Sprintf("%s/.botelho.json", home)
	contents, err := ioutil.ReadFile(name)
	if err != nil {
		log.Fatalf("Could not read %s: %q", name, err)
	}

	config := Config{}
	err = json.Unmarshal([]byte(contents), &config)
	if err != nil {
		log.Fatalf("Could not parse configuration file %s: %q", name, err)
	}

	config.GroupId, err = strconv.ParseInt(config.GroupIdStr, 10, 64)
	if err != nil {
		log.Fatalf("Could not parse group id: %q", err)
	}

	return config
}

func fetch(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	return body, nil
}

func fetchWho() (Whois, error) {
	body, err := fetch("https://lhc.net.br/spacenet.json?whois")
	if err != nil {
		return Whois{}, err
	}

	whois := Whois{}
	err = json.Unmarshal([]byte(body), &whois)
	if err != nil {
		return Whois{}, err
	}

	whois.Who = removeDuplicate(whois.Who)
	return whois, nil
}

func removeDuplicate(xs []string) []string {
	found := make(map[string]bool)
	j := 0
	for i, x := range xs {
		if !found[x] {
			found[x] = true
			xs[j] = xs[i]
			j++
		}
	}
	return xs[:j]
}

func fetchStatus(status chan Status) {
	old_status := Status{false, 0}
	for {
		body, err := fetch("https://lhc.net.br/spacenet.json")
		if err != nil {
			log.Printf("Could not get status JSON (%s), waiting 5 minutes.", err)
			time.Sleep(5 * time.Minute)
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

func pizzaPorPessoa(n_pessoas, n_pedacos float64) float64 {
	var pedacos_por_pessoa float64

	if n_pedacos > 20 {
		pedacos_por_pessoa = 2.0
	} else {
		pedacos_por_pessoa = 3.0
	}

	return math.Ceil((pedacos_por_pessoa * (n_pessoas + 1.0)) / n_pedacos)
}

func diaDaSemana(weekday time.Weekday) string {
	switch weekday {
	default:
		return "🦆 "
	case time.Sunday:
		return "Domingo"
	case time.Monday:
		return "Segunda"
	case time.Tuesday:
		return "Terça"
	case time.Wednesday:
		return "Quarta"
	case time.Thursday:
		return "Quinta"
	case time.Friday:
		return "Sexta"
	case time.Saturday:
		return "Sábado"
	}
}

func imprimeEvento(chat telebot.Chat, bot *telebot.Bot, event *ics.Event, agora time.Time) bool {
	if event.GetStart().Before(agora) {
		return false
	}
	if event.GetEnd().Before(agora) {
		bot.SendMessage(chat, "Tá rolando agora: "+event.GetSummary(), nil)
	} else {
		var proximoEvento string

		if event.GetStart().YearDay() == agora.YearDay() && event.GetStart().Year() == agora.Year() {
			proximoEvento = fmt.Sprintf("Hoje tem \"%s\" às %02d:%02d",
				event.GetSummary(),
				event.GetStart().Hour(), event.GetStart().Minute())
		} else {
			proximoEvento = fmt.Sprintf("Vai rolar \"%s\" no dia %02d/%02d (%s) às %02d:%02d",
				event.GetSummary(),
				event.GetStart().Day(),
				event.GetStart().Month(),
				diaDaSemana(event.GetStart().Weekday()),
				event.GetStart().Hour(), event.GetStart().Minute())
		}
		bot.SendMessage(chat, proximoEvento, nil)
	}
	return true
}

func processaIcs(chat telebot.Chat, bot *telebot.Bot, agora time.Time, url string) bool {
	parser := ics.New()
	defer parser.Wait()

	parser.GetInputChan() <- url
	for event := range parser.GetOutputChan() {
		if imprimeEvento(chat, bot, event, agora) {
			return true
		}
		if parser.Done() {
			break
		}
	}

	return false
}

func paraDataHora(data, hora time.Time) time.Time {
	return time.Date(data.Year(), data.Month(), data.Day(), hora.Hour(), hora.Minute(), 0, 0, time.UTC)
}

func processaRecorrente(chat telebot.Chat, bot *telebot.Bot, agora time.Time) bool {
	// * [[Oficina de Computacao Cognitiva]] ''12/09/2017 das 19:00 às 22:30: Oficina com IBM Bluemix''
	eventoRe := regexp.MustCompile(`^\s*\*\s*\[\[([\p{Latin}\s|_0-9/]+)\]\]\s*\'\'(\d{1,2}/\d{1,2}/\d{4})[\p{Latin}\s]+(\d{1,2}:\d{1,2})[\p{Latin}\s]+(\d{1,2}:\d{1,2})[:\s-]*([\p{Latin}\s]+)?\'\'`)

	recorrente, err := fetch("https://lhc.net.br/w/index.php?title=Pr%C3%B3ximos_Eventos&action=raw")
	if err != nil {
		log.Printf("Enquanto pegava lista de evento recorrente: %s", err)
		return false
	}

	for _, line := range strings.Split(string(recorrente), "\n") {
		if match := eventoRe.FindStringSubmatch(line); match != nil {
			nome := strings.TrimSpace(match[1])
			data, err := time.Parse("02/01/2006", match[2])
			if err != nil {
				continue
			}
			horaInicio, err := time.Parse("15:04", match[3])
			if err != nil {
				continue
			}
			horaFim, err := time.Parse("15:04", match[4])
			if err != nil {
				continue
			}

			if strings.Contains(nome, "|") {
				nome = strings.Split(nome, "|")[1]
			}

			if match[5] != "" && match[5] != nome {
				nome = nome + " (" + match[5] + ")"
			}

			dataInicio := paraDataHora(data, horaInicio)
			dataFim := paraDataHora(data, horaFim)
			evento := ics.NewEvent().SetStart(dataInicio).SetEnd(dataFim).SetSummary(nome)

			if imprimeEvento(chat, bot, evento, agora) {
				return true
			}
		}
	}

	log.Printf("Nao peguei nenhum evento recorrente")
	return false
}

func pegaEventoTimeout(f func() bool) bool {
	c := make(chan bool, 1)
	go func() { c <- f() }()
	select {
	case res := <-c:
		return res
	case <-time.After(3 * time.Second):
		return false
	}
}

func pegaEventos(chat telebot.Chat, bot *telebot.Bot, agora time.Time) {
	ics := []string{
		"https://lhc.net.br/lhc.ics",
		"https://www.meetup.com/LabHackerCampinas/events/ical/",
		"https://www.facebook.com/ical/u.php?uid=100009917924593&key=AQBjZm11n-K_NCbW",
	}

	for _, uri := range ics {
		f := func() bool {
			return processaIcs(chat, bot, agora, uri)
		}
		if pegaEventoTimeout(f) {
			return
		}
	}

	if pegaEventoTimeout(func() bool { return processaRecorrente(chat, bot, agora) }) {
		return
	}

	bot.SendMessage(chat, "Pegar a lista de eventos demorou demais e eu desisti. Eu procurei no calendário público, MeetUp, Facebook, e no Wiki do LHC.", nil)
}

func atualizaThingspeak() {
	who, err := fetchWho()
	if err != nil {
		log.Printf("Could not fetch who's in the space: %s", err)
		return
	}

	var field1, field2, field3 int
	if len(who.Who) > 0 {
		field1 = 1
	} else {
		field1 = 0
	}
	field2 = len(who.Who)
	field3 = who.UnknownMacs

	ts_url := fmt.Sprintf("https://api.thingspeak.com/update?api_key=%s&field1=%d&field2=%d&field3=%d",
		config.ThingSpeakAPIKey, field1, field2, field3)

	_, err = fetch(ts_url)
	if err != nil {
		log.Printf("Error while updating thingspeak: %q", err)
	}
}

func getRandomSpaceEmoji() string {
	emojis := []string{"🌌", "🚀", "🛸", "🛰"}
	return emojis[rand.Intn(len(emojis))]
}

func progressBar(current, max float64) string {
	width := 15

	painted := int(float64(width) * current / max)
	bar := ""
	for i := 0; i < painted; i++ {
		bar = bar + "█"
	}
	for i := 0; i < width-painted; i++ {
		bar = bar + "░"
	}

	return bar
}

func pegaGrana() (float64, float64, error) {
	status, err := fetch("http://beta.lhc.rennerocha.com/status")
	if err != nil {
		return 0, 0, err
	}

	financas := Financas{}
	err = json.Unmarshal([]byte(status), &financas)
	if err != nil {
		return 0, 0, err
	}

	income, err := strconv.ParseFloat(financas.ActualIncomes, 64)
	if err != nil {
		return 0, 0, err
	}

	expenses, err := strconv.ParseFloat(financas.RegularExpensesEstimate, 64)
	if err != nil {
		return 0, 0, err
	}

	return income, expenses, nil
}

func mostraGrana(chat telebot.Chat, bot *telebot.Bot) {
	income, expenses, err := pegaGrana()
	if err != nil {
		bot.SendMessage(chat, "Não consegui pegar as finanças do LHC", nil)
		return
	}

	msg := ""
	if income > expenses {
		msg = fmt.Sprintf("Temos fluxo positivo de caixa esse mês! Recebemos R$%.2f de %%.2f", income, expenses)
	} else {
		msg = fmt.Sprintf("Este mês recebemos R$%.2f de R$%.2f.\n\n%s", income, expenses, progressBar(income, expenses))
	}

	bot.SendMessage(chat, msg, nil)
}

var ultimoMesMonitorGrana *time.Time

func monitoraGrana(chat telebot.Chat, bot *telebot.Bot) {
	income, expenses, err := pegaGrana()
	if err != nil {
		return
	}

	if income < expenses {
		return
	}

	t := time.Now()
	if ultimoMesMonitorGrana == nil || ultimoMesMonitorGrana.Month() != t.Month() || ultimoMesMonitorGrana.Year() != t.Year() {
		ultimoMesMonitorGrana = &t
		bot.SendMessage(chat, "🎉 Conseguimos a grana pra manter o LHC aberto esse mês!  Mais detalhes: /grana", nil)
	}
}

func main() {
	config = loadConfig()

	bot, err := telebot.NewBot(config.TelegramAPIKey)
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
	pizzaMsg := "Quantas pessoas vão querer pizza? 🍕"

	saoPaulo, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("Could not load location for timezone purposes")
	}

	dest := telebot.Chat{config.GroupId, telebot.ChatType(config.GroupType), config.GroupName, "", "", ""}

	for {
		select {
		case <-time.After(1 * time.Hour):
			go monitoraGrana(dest, bot)

		case <-time.After(10 * time.Minute):
			go atualizaThingspeak()

		case s := <-status:
			go func(chat telebot.Chat, bot *telebot.Bot) {
				var whoOpened string
				if s.Open {
					estado = "aberto🔓"
					who, err := fetchWho()
					if err == nil {
						whoOpened = strings.Join(who.Who, ", ")
					}
				} else {
					estado = "fechado🔒"
				}
				tm := time.Unix(s.LastChange, 0).In(saoPaulo)
				desde = tm.Format(time.ANSIC)

				fmt.Printf("Mudança de estado: %s em %s\n", estado, desde)

				ultimoEstado = "O LHC está " + estado + " desde " + desde + "."
				if whoOpened != "" {
					bot.SendMessage(chat, "O LHC foi aberto🔓 por "+whoOpened+" às "+desde+".", nil)
				} else {
					bot.SendMessage(dest, ultimoEstado, nil)
				}
			}(dest, bot)

		case message := <-messages:
			fmt.Printf("Recebi mensagem: %s de: %s (%v)\n", message.Text,
				message.Chat, message.Chat)
			if strings.HasPrefix(message.Text, "/status") {
				bot.SendMessage(message.Chat, ultimoEstado, nil)
			} else if strings.HasPrefix(message.Text, "/historico") {
				url := fmt.Sprintf("Para ver o histórico, acesse: https://thingspeak.com/channels/%d", config.ThingSpeakChannel)
				bot.SendMessage(message.Chat, url, nil)
			} else if strings.HasPrefix(message.Text, "/grana") {
				go mostraGrana(message.Chat, bot)
			} else if strings.HasPrefix(message.Text, "/quem") {
				go func(chat telebot.Chat, bot *telebot.Bot) {
					who, err := fetchWho()
					var msg string

					emoji := getRandomSpaceEmoji()

					if err != nil {
						msg = "Não consegui pegar a lista de membros no espaço" + emoji
					} else {
						if len(who.Who) > 0 {
							msg = "Pessoas conhecidas no espaço" + emoji + ": " + strings.Join(who.Who, ", ")
						} else {
							msg = "Não tem nenhuma pessoa conhecida lá"
						}
						if who.UnknownMacs == 1 {
							msg = msg + ". Mais uma pessoa desconhecida"
						} else if who.UnknownMacs > 1 {
							msg = msg + fmt.Sprintf(". Mais %d pessoas desconhecidas", who.UnknownMacs)
						}
					}

					bot.SendMessage(chat, msg, nil)
				}(message.Chat, bot)
			} else if strings.HasPrefix(message.Text, "/quando") {
				go pegaEventos(message.Chat, bot, time.Now().In(saoPaulo))
			} else if strings.HasPrefix(message.Text, "/pizza") {
				bot.SendMessage(message.Chat, pizzaMsg, &telebot.SendOptions{
					ReplyMarkup: telebot.ReplyMarkup{
						ForceReply:      true,
						Selective:       false,
						ResizeKeyboard:  true,
						OneTimeKeyboard: true,
						CustomKeyboard: [][]string{
							[]string{"1", "2", "3"},
							[]string{"4", "5", "6"},
							[]string{"7", "8", "9"},
							[]string{"10", "11", "12"},
						},
					},
				})
			} else if message.IsReply() && strings.HasPrefix(message.ReplyTo.Text, pizzaMsg) {
				n_pessoas, err := strconv.ParseFloat(message.Text, 64)

				var msg string
				if err != nil {
					msg = "Não entendi a quantidade de pizzas: " + message.Text
				} else if n_pessoas >= 1 && n_pessoas <= 100 {
					msg = fmt.Sprintf("Para %.0f pessoas, compre %.0f pizzas de 8 🍕.", n_pessoas, pizzaPorPessoa(n_pessoas, 8))

					/* Promoções */
					/* Promoções ainda válidas?
					switch time.Now().In(saoPaulo).Weekday() {
					case time.Monday:
						msg = msg + " Promoção na Didio hoje: pizza de pepperoni R$15 mais barato. Peça no site: http://didio.com.br/."
					case time.Thursday:
						msg = msg + " Promoção no Habib's hoje: pizza grande a R$16,90. Peça no site http://www.deliveryhabibs.com.br/."
					}*/

					/* Opções de pizza além das promoções */
					if n_pessoas > 7 {
						msg = msg + " Para a quantidade de pessoas, também tem a pizza de 60cm da http://mpizza.com.br/: "
						msg = msg + fmt.Sprintf("cada uma tem 22 pedaços, então recomendo pedir %.0f mega pizzas.", pizzaPorPessoa(n_pessoas, 22))
					} else {
						msg = msg + " Uma opção é pedir na Penedos e pegar um imã/pizza. 8 deles trocam por uma pizza: http://penedos.com.br/catalog ou ligue 3396-5002."
					}
				} else if n_pessoas == 0 {
					msg = "Para nenhuma pessoa, é melhor nem comprar pizza"
				} else if n_pessoas < 0 {
					msg = "Número negativo de pizzas? Não viramos uma pizzaria"
				} else {
					msg = "Mais que 100 pessoas no LHC? Isso vai dar overflow nos meus cálculos, se vira aí."
				}

				bot.SendMessage(message.Chat, msg, &telebot.SendOptions{
					ReplyMarkup: telebot.ReplyMarkup{
						HideCustomKeyboard: true,
					},
				})
			}
		}
	}
}
