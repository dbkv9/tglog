package main

import (
	"fmt"
	"github.com/alexflint/go-arg"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/nxadm/tail"
	"github.com/oriser/regroup"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"log"
	"net/url"
	"regexp"
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
	Log       string
	Host      string
	Webserver string
	Format    string
	Variables []string
	Regexp    *regroup.ReGroup
}

type Config struct {
	Projects map[string]ProjectConfig `yaml:",flow"`
}

const MSG_TEMPLATE = `
<b>%s [%s]</b>

<b>STATUS CODE</b>: %d
<b>URI</b>: %s [%s]
<b>METHOD</b>: %s

##################
<pre>%s</pre>`

func parse(name string, tailer *tail.Tail, cfg *ProjectConfig, ch chan Row) {
	for line := range tailer.Lines {
		if line.Text == "" {
			continue
		}

		row := &Row{}
		err := cfg.Regexp.MatchToTarget(line.Text, row)

		if err != nil {
			log.Fatal(err)
			continue
		}

		// skip empty requests when client connection is broken
		if row.Request == "-" || row.Status == 000 {
			continue
		}

		t, _ := time.Parse("02/Jan/2006:15:04:05 -0700", row.LocalTime)
		row.LocalTimeP = t

		// split request string and fill RequestP struct
		// 0 - method like POST, GET
		// 1 - request URI
		// 2 - protocol like HTTP1.0 / HTTP2.0
		request_p := strings.Split(row.Request, " ")
		row.RequestP.Method = request_p[0]
		row.RequestP.Uri, _ = url.JoinPath(cfg.Host, request_p[1])
		row.RequestP.Protocol = request_p[2]

		row.ProjectName = name

		ch <- *row
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

func main() {
	var args struct {
		Config string `arg:"required"`
	}

	arg.MustParse(&args)
	cfg := readcfg(args.Config)

	re := regexp.MustCompile(`\$\w+`)

	ch_row := make(chan Row)

	for name, project_cfg := range cfg.Projects {
		// prepare variables for row parser
		project_cfg.Variables = re.FindAllString(project_cfg.Format, -1)

		// escapes for brackets
		r := strings.NewReplacer("[", `\[`, "]", `\]`)
		re_pattern := r.Replace(project_cfg.Format)

		for _, x := range project_cfg.Variables {
			re_pattern = strings.Replace(re_pattern, x, "(?P<"+strings.Replace(x, "$", "", -1)+">.*?)", -1)
		}

		// prepare regexp pattern for row parser
		project_cfg.Regexp = regroup.MustCompile(re_pattern)

		t, err := tail.TailFile(project_cfg.Log,
			tail.Config{Follow: true, ReOpen: true, Poll: true})

		if err != nil {
			log.Panic(err)
		}

		go parse(name, t, &project_cfg, ch_row)
	}

	now := time.Now()

	bot, err := tgbotapi.NewBotAPI("6575068059:AAG6zSPg0P2qK9-KmFglVZ8q5fLYD6jeUSM")
	if err != nil {
		log.Panic(err)
	}

	for {
		select {
		case row := <-ch_row:
			// skip older rows
			if now.Compare(row.LocalTimeP) == -1 {
				if !slices.Contains([]uint{200, 301, 302, 304, 404}, row.Status) {
					markup := fmt.Sprintf(MSG_TEMPLATE,
						row.LocalTimeP, row.ProjectName, row.Status,
						row.RequestP.Uri, row.RequestP.Protocol,
						row.RequestP.Method, row.UserAgent)

					msg := tgbotapi.NewMessage(187643882, markup)
					msg.ParseMode = "html"
					msg.DisableWebPagePreview = true
					bot.Send(msg)
				}
			}
		}
	}
}
