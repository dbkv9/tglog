/*
 * todo:
 * - refactor config access by key-val
 * - remove DB clover
 */

package main

import (
	"bufio"
	"fmt"
	"github.com/alexflint/go-arg"
	"github.com/go-co-op/gocron"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/nxadm/tail"
	"github.com/oriser/regroup"
	"github.com/xuri/excelize/v2"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
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
	Total     int
	Total_5xx int
	Total_4xx int
	Total_3xx int
	Total_2xx int
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

func parse(line string, cfg ProjectConfig) (Row, bool) {
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
	request_p := strings.Split(row.Request, " ")

	// skip invalid requests
	if len(request_p) < 3 {
		return Row{}, false
	}
	row.RequestP.Method = request_p[0]
	row.RequestP.Uri, _ = url.JoinPath(cfg.Host, request_p[1])
	row.RequestP.Protocol = request_p[2]

	return *row, true
}

func watch(name string, tailer *tail.Tail, cfg ProjectConfig, ch chan Row) {
	for line := range tailer.Lines {
		if line.Text == "" {
			continue
		}

		if row, ok := parse(line.Text, cfg); ok {
			row.ProjectName = name
			ch <- row
		}

	}
}

func readcfg(path string) Config {
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

func sendreport(bot tgbotapi.BotAPI, chat_id int64, name string, daily_history *map[string][]DailyRow) {
	report := Report{}

	for _, row := range (*daily_history)[name] {
		status := row.Status

		report.Total++

		if status >= 200 && status <= 299 {
			report.Total_2xx++
		} else if status >= 300 && status <= 399 {
			report.Total_3xx++
		} else if status >= 400 && status <= 499 {
			report.Total_4xx++
		} else if status >= 500 && status <= 599 {
			report.Total_5xx++
		}
	}

	markup := fmt.Sprintf(REPORT_TEMPLATE,
		name, time.Now().Format(time.RFC1123), report.Total_5xx,
		report.Total_4xx, report.Total_3xx, report.Total_2xx,
		report.Total)

	tg_send_message(bot, chat_id, markup)

	(*daily_history)[name] = []DailyRow{}
}

func tg_send_message(bot tgbotapi.BotAPI, chat_id int64, markup string) {
	msg := tgbotapi.NewMessage(chat_id, markup)
	msg.ParseMode = "html"
	msg.DisableWebPagePreview = true
	bot.Send(msg)
}

func is_today(localtime time.Time) bool {
	now := time.Now()
	day_start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	day_end := day_start.Add(time.Hour * 24)

	return (day_start.Compare(localtime) == -1 || day_start.Compare(localtime) == 0) &&
		(day_end.Compare(localtime) == 1)
}

func tglistner(bot *tgbotapi.BotAPI, cfg_reverse ConfigReverse) {
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
			if project_cfg, ok := cfg_reverse[update.Message.Chat.ID]; ok {
				file, err := os.Open(project_cfg.Log)
				if err != nil {
					log.Panic(err)
				}

				defer file.Close()

				project_cfg.ParseRegexp = get_compiled_regexp(project_cfg.Format)

				rows := []Row{}

				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					if row, ok := parse(scanner.Text(), project_cfg); ok {
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
func get_cfg_reverse_map(projects map[string]ProjectConfig) ConfigReverse {
	cfg := ConfigReverse{}

	for _, project_cfg := range projects {
		cfg[project_cfg.TgChat] = project_cfg
	}

	return cfg
}

func get_compiled_regexp(format string) *regroup.ReGroup {
	re := regexp.MustCompile(`\$\w+`)

	// prepare variables for row parser
	variables := re.FindAllString(format, -1)

	// escapes for brackets
	r := strings.NewReplacer("[", `\[`, "]", `\]`)
	re_pattern := r.Replace(format)

	for _, x := range variables {
		re_pattern = strings.Replace(re_pattern, x, "(?P<"+strings.Replace(x, "$", "", -1)+">.*?)", -1)
	}

	// prepare regexp pattern for row parser
	re_compiled := regroup.MustCompile(re_pattern)

	return re_compiled
}

func main() {
	var args struct {
		Config string `arg:"required"`
	}

	arg.MustParse(&args)
	cfg := readcfg(args.Config)

	ch_row := make(chan Row)

	// daily history of requests
	daily_history := map[string][]DailyRow{}

	// cron scheduler pool
	cron_pool := map[string]*gocron.Scheduler{}

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

	cfg_reverse := get_cfg_reverse_map(cfg.Projects)
	// init tg listners
	go tglistner(tgbot, cfg_reverse)

	for name, project_cfg := range cfg.Projects {
		// get complied regexp from log format for parse row's
		project_cfg.ParseRegexp = get_compiled_regexp(project_cfg.Format)

		// init cron for project's report and send it
		cron_pool[name] = gocron.NewScheduler(time.UTC)
		cron_pool[name].Cron(project_cfg.ReportSchedule).Do(sendreport, *tgbot, cfg.Projects[name].TgChat, name, &daily_history)
		cron_pool[name].StartAsync()

		// init tail for each log-file
		t, err := tail.TailFile(project_cfg.Log,
			tail.Config{Follow: true, ReOpen: true, Poll: true})

		if err != nil {
			log.Panic(err)
		}

		go watch(name, t, project_cfg, ch_row)
	}

	now := time.Now()

	for {
		select {
		case row := <-ch_row:
			if is_today(row.LocalTimeP) {
				daily := DailyRow{row.LocalTimeP, row.Status}
				daily_history[row.ProjectName] = append(daily_history[row.ProjectName], daily)
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

					tg_send_message(*tgbot, cfg.Projects[row.ProjectName].TgChat, markup)
				}
			}
		}
	}
}
