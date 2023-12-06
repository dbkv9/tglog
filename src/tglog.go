package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/go-co-op/gocron"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/nxadm/tail"
	"github.com/oriser/regroup"
	"github.com/xuri/excelize/v2"
	"gopkg.in/yaml.v3"
)

type Row struct {
	RemoteAddr string    `regroup:"remote_addr"`
	RemoteUser string    `regroup:"remote_user"`
	LocalTime  string    `regroup:"time_local"`
	LocalTimeP time.Time // LocalTime converted to time.Time struct
	Request    string    `regroup:"request"`
	RequestP   struct {  // Request parserd to structure
		Method   string // Like GET/POST/PUT
		Uri      string // Request URI
		Protocol string // Like HTTP, HTTP 2.0
	}
	Status      uint   `regroup:"status"`
	Bytes       uint   `regroup:"body_bytes_sent"`
	Referer     string `regroup:"http_referer"`
	UserAgent   string `regroup:"http_user_agent"`
	ProjectName string
}

type ProjectConfig struct {
	Log            string
	Host           string
	ReportSchedule string
	Webserver      string
	Format         string
	ParseRegexp    *regroup.ReGroup
	TgChat         int64
}

type Config struct {
	Projects map[string]ProjectConfig `yaml:",flow"`
	Tgtoken  string
}

type ConfigReverse map[int64]ProjectConfig

type DailyRow struct {
	LocalTime time.Time
	Status    uint
}

type Report struct {
	Total    int
	Total5xx int
	Total4xx int
	Total3xx int
	Total2xx int
}

const MSG_TEMPLATE = `
%s <b>%d</b>: %s

<b>IP</b>: %s
<b>DATE</b>: %s
<b>METHOD</b>: %s / %s

<pre>%s</pre>`

const REPORT_TEMPLATE = `
ℹ️ <b>REPORT %s - %s</b>

<b>5xx</b>:  %d
<b>4xx</b>:  %d
<b>3xx</b>:  %d
<b>2xx</b>:  %d

<b>TOTAL</b>: %d
`

func ParseLine(line string, cfg ProjectConfig) (Row, bool) {
	row := &Row{}
	err := cfg.ParseRegexp.MatchToTarget(line, row)

	if err != nil {
		log.Println(err)
		return Row{}, false
	}

	// skip empty requests when client connection is broken
	if row.Request == "-" || row.Status == 000 {
		return Row{}, false
	}

	t, _ := time.Parse("02/Jan/2006:15:04:05 -0700", row.LocalTime)
	row.LocalTimeP = t

	// split request string and fill RequestP struct
	// 0 - method like POST, GET
	// 1 - request URI
	// 2 - protocol like HTTP1.0 / HTTP2.0
	requestParsed := strings.Split(row.Request, " ")

	// skip invalid requests
	if len(requestParsed) < 3 {
		return Row{}, false
	}
	row.RequestP.Method = requestParsed[0]

	parsed_url, _ := url.Parse(requestParsed[1])
	if parsed_url.Host == "" {
		row.RequestP.Uri, _ = url.JoinPath(cfg.Host, requestParsed[1])
	} else {
		row.RequestP.Uri = requestParsed[1]
	}
	row.RequestP.Protocol = requestParsed[2]

	return *row, true
}

func WatchLog(name string, tailer *tail.Tail, cfg ProjectConfig, ch chan Row) {
	for line := range tailer.Lines {
		if line.Text == "" {
			continue
		}

		if row, ok := ParseLine(line.Text, cfg); ok {
			row.ProjectName = name
			ch <- row
		}

	}
}

func ReadConfig(path string) Config {
	conf := Config{}
	content, err := ioutil.ReadFile(path)

	if err != nil {
		log.Fatal(err)
	}

	err = yaml.Unmarshal([]byte(content), &conf)
	if err != nil {
		log.Fatal(err)
	}

	return conf
}

func SendReport(bot tgbotapi.BotAPI, chat_id int64, name string, dailyHistory *map[string][]DailyRow) {
	report := Report{}

	for _, row := range (*dailyHistory)[name] {
		status := row.Status

		report.Total++

		if status >= 200 && status <= 299 {
			report.Total2xx++
		} else if status >= 300 && status <= 399 {
			report.Total3xx++
		} else if status >= 400 && status <= 499 {
			report.Total4xx++
		} else if status >= 500 && status <= 599 {
			report.Total5xx++
		}
	}

	markup := fmt.Sprintf(REPORT_TEMPLATE,
		name, time.Now().Format(time.RFC1123), report.Total5xx,
		report.Total4xx, report.Total3xx, report.Total2xx,
		report.Total)

	SendMessage(bot, chat_id, markup)

	(*dailyHistory)[name] = []DailyRow{}
}

func SendMessage(bot tgbotapi.BotAPI, chat_id int64, markup string) {
	msg := tgbotapi.NewMessage(chat_id, markup)
	msg.ParseMode = "html"
	msg.DisableWebPagePreview = true
	bot.Send(msg)
}

func IsToday(localtime time.Time) bool {
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.Add(time.Hour * 24)

	return (dayStart.Compare(localtime) == -1 || dayStart.Compare(localtime) == 0) &&
		(dayEnd.Compare(localtime) == 1)
}

func BotListner(bot *tgbotapi.BotAPI, cfgReverse ConfigReverse) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		if !update.Message.IsCommand() {
			continue
		}

		if update.Message.Command() == "export" {
			if projectCfg, ok := cfgReverse[update.Message.Chat.ID]; ok {
				file, err := os.Open(projectCfg.Log)
				if err != nil {
					log.Panic(err)
				}

				defer file.Close()

				projectCfg.ParseRegexp = CompileRegexp(projectCfg.Format)

				rows := []Row{}

				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					if row, ok := ParseLine(scanner.Text(), projectCfg); ok {
						rows = append(rows, row)
					}
				}

				if err := scanner.Err(); err != nil {
					log.Panic(err)
				}

				if len(rows) > 0 {
					f := excelize.NewFile()
					defer func() {
						if err := f.Close(); err != nil {
							fmt.Println(err)
						}
					}()

					index, err := f.NewSheet("all")
					if err != nil {
						fmt.Println(err)
						return
					}

					f.SetCellValue("all", "A1", "RemoteAddr")
					f.SetCellValue("all", "B1", "RemoteUser")
					f.SetCellValue("all", "C1", "LocalTime")
					f.SetCellValue("all", "D1", "RequestMethod")
					f.SetCellValue("all", "E1", "RequestUri")
					f.SetCellValue("all", "F1", "RequestProtocol")
					f.SetCellValue("all", "G1", "Status")
					f.SetCellValue("all", "H1", "Bytes")
					f.SetCellValue("all", "I1", "Referer")
					f.SetCellValue("all", "J1", "UserAgent")

					for i, row := range rows {
						f.SetCellValue("all", "A"+strconv.Itoa(i+2), row.RemoteAddr)
						f.SetCellValue("all", "B"+strconv.Itoa(i+2), row.RemoteUser)
						f.SetCellValue("all", "C"+strconv.Itoa(i+2), row.LocalTime)
						f.SetCellValue("all", "D"+strconv.Itoa(i+2), row.RequestP.Method)
						f.SetCellValue("all", "E"+strconv.Itoa(i+2), row.RequestP.Uri)
						f.SetCellValue("all", "F"+strconv.Itoa(i+2), row.RequestP.Protocol)
						f.SetCellValue("all", "G"+strconv.Itoa(i+2), row.Status)
						f.SetCellValue("all", "H"+strconv.Itoa(i+2), row.Bytes)
						f.SetCellValue("all", "I"+strconv.Itoa(i+2), row.Referer)
						f.SetCellValue("all", "J"+strconv.Itoa(i+2), row.UserAgent)
					}

					f.SetActiveSheet(index)

					now := time.Now()
					filename := fmt.Sprintf("export_%d.xlsx", now.Unix())
					if err := f.SaveAs(filename); err != nil {
						log.Fatal(err)
					}

					msg := tgbotapi.NewDocument(update.Message.Chat.ID, tgbotapi.FilePath(filename))
					bot.Send(msg)
				}
			}
		}

	}
}

// return reverse map of cfg's by tg chat_id
func GetConfigReverseMap(projects map[string]ProjectConfig) ConfigReverse {
	cfg := ConfigReverse{}

	for _, projectCfg := range projects {
		cfg[projectCfg.TgChat] = projectCfg
	}

	return cfg
}

func CompileRegexp(format string) *regroup.ReGroup {
	re := regexp.MustCompile(`\$\w+`)

	// prepare variables for row parser
	variables := re.FindAllString(format, -1)

	// escapes for brackets
	r := strings.NewReplacer("[", `\[`, "]", `\]`)
	rePattern := r.Replace(format)

	for _, x := range variables {
		rePattern = strings.Replace(rePattern, x, "(?P<"+strings.Replace(x, "$", "", -1)+">.*?)", -1)
	}

	// prepare regexp pattern for row parser
	re_compiled := regroup.MustCompile(rePattern)

	return re_compiled
}

func main() {
	var args struct {
		Config string `default:"/usr/local/etc/tglog/config.yaml"`
	}

	arg.MustParse(&args)
	cfg := ReadConfig(args.Config)

	chRow := make(chan Row)

	// daily history of requests
	dailyHistory := map[string][]DailyRow{}

	// cron scheduler pool
	cronPool := map[string]*gocron.Scheduler{}

	// tg bot init
	tgbot, err := tgbotapi.NewBotAPI(cfg.Tgtoken)
	if err != nil {
		log.Panic(err)
	}

	commands_cfg := tgbotapi.NewSetMyCommands(tgbotapi.BotCommand{
		Command:     "export",
		Description: "Export log to Excel",
	})
	tgbot.Request(commands_cfg)

	cfgReverse := GetConfigReverseMap(cfg.Projects)
	// init tg listners
	go BotListner(tgbot, cfgReverse)

	for name, projectCfg := range cfg.Projects {
		// get complied regexp from log format for parse row's
		projectCfg.ParseRegexp = CompileRegexp(projectCfg.Format)

		// init cron for project's report and send it
		cronPool[name] = gocron.NewScheduler(time.UTC)
		cronPool[name].Cron(projectCfg.ReportSchedule).Do(SendReport, *tgbot, cfg.Projects[name].TgChat, name, &dailyHistory)
		cronPool[name].StartAsync()

		// init tail for each log-file
		t, err := tail.TailFile(projectCfg.Log,
			tail.Config{Follow: true, ReOpen: true, Poll: true})

		if err != nil {
			log.Panic(err)
		}

		go WatchLog(name, t, projectCfg, chRow)
	}

	now := time.Now()

	for {
		select {
		case row := <-chRow:
			if IsToday(row.LocalTimeP) {
				daily := DailyRow{row.LocalTimeP, row.Status}
				dailyHistory[row.ProjectName] = append(dailyHistory[row.ProjectName], daily)
			}

			// skip older rows
			if now.Compare(row.LocalTimeP) == -1 {
				if row.Status >= 500 {
					emoji := ""
					if row.Status >= 400 {
						emoji = "🟨"
					}

					if row.Status >= 500 {
						emoji = "🟥"
					}

					markup := fmt.Sprintf(MSG_TEMPLATE,
						emoji, row.Status, row.RequestP.Uri,
						row.RemoteAddr, row.LocalTimeP, row.RequestP.Method,
						row.RequestP.Protocol, row.UserAgent)

					SendMessage(*tgbot, cfg.Projects[row.ProjectName].TgChat, markup)
				}
			}
		}
	}
}
