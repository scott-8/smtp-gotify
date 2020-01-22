package main

import (
	"errors"
	"fmt"
	"github.com/flashmob/go-guerrilla"
	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/log"
	"github.com/flashmob/go-guerrilla/mail"
	"github.com/jhillyerd/enmime"
	"github.com/urfave/cli/v2"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type SmtpConfig struct {
	smtpListen      string
	smtpPrimaryHost string
}

type GofityConfig struct {
	gotifyPriority  string
	gotifyAPIToken  string
	gotifyURL       string
	titleTemplate   string
	messageTemplate string
}

func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		panic(fmt.Sprintf("Unable to detect hostname: %s", err))
	}
	return hostname
}

func main() {
	app := cli.NewApp()
	app.Name = "smtp-gotify"
	app.Usage = "A small program which listens for SMTP and sends " +
		"all incoming Email messages to your Gotify server."
	app.Version = "UNKNOWN_RELEASE"
	app.Action = func(c *cli.Context) error {
		// Required flags are not supported, see https://github.com/urfave/cli/issues/85
		if !c.IsSet("gotify-url") {
			return cli.NewExitError("Gotify URL is missing. See `--help`", 2)
		}
		if !c.IsSet("gotify-api-token") {
			return cli.NewExitError("Gotify API token is missing. See `--help`", 2)
		}
		smtpConfig := &SmtpConfig{
			smtpListen:      c.String("smtp-listen"),
			smtpPrimaryHost: c.String("smtp-primary-host"),
		}
		gotifyConfig := &GofityConfig{
			gotifyPriority:  c.String("gotify-priority"),
			gotifyAPIToken:  c.String("gotify-api-token"),
			gotifyURL:       c.String("gotify-url"),
			titleTemplate:   c.String("title-template"),
			messageTemplate: c.String("message-template"),
		}
		d, err := SmtpStart(smtpConfig, gotifyConfig)
		if err != nil {
			panic(fmt.Sprintf("start error: %s", err))
		}
		sigHandler(d)
		return nil
	}
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "smtp-listen",
			Value:   "127.0.0.1:2525",
			Usage:   "SMTP: TCP address to listen to",
			EnvVars: []string{"SG_SMTP_LISTEN"},
		},
		&cli.StringFlag{
			Name:    "smtp-primary-host",
			Value:   GetHostname(),
			Usage:   "SMTP: primary host",
			EnvVars: []string{"SG_SMTP_PRIMARY_HOST"},
		},
		&cli.StringFlag{
			Name:    "gotify-priority",
			Value:   "5",
			Usage:   "Gotify message priority",
			EnvVars: []string{"GOFITY_PRIORITY"},
		},
		&cli.StringFlag{
			Name:    "gotify-api-token",
			Usage:   "Gotify API token",
			EnvVars: []string{"GOTIFY_TOKEN"},
		},
		&cli.StringFlag{
			Name:    "gotify-url",
			Usage:   "Gotify server URL",
			EnvVars: []string{"GOTIFY_URL"},
		},
		&cli.StringFlag{
			Name:    "title-template",
			Usage:   "Gotify notification title template",
			Value:   "{subject}",
			EnvVars: []string{"GOTIFY_TITLE_TEMPLATE"},
		},
		&cli.StringFlag{
			Name:    "message-template",
			Usage:   "Gotify notification message template",
			Value:   "{body}",
			EnvVars: []string{"GOTIFY_MESSAGE_TEMPLATE"},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		os.Exit(1)
	}
}

func SmtpStart(
	smtpConfig *SmtpConfig, gotifyConfig *GofityConfig) (guerrilla.Daemon, error) {

	cfg := &guerrilla.AppConfig{LogFile: log.OutputStdout.String()}

	cfg.AllowedHosts = []string{"."}

	sc := guerrilla.ServerConfig{
		ListenInterface: smtpConfig.smtpListen,
		IsEnabled:       true,
	}
	cfg.Servers = append(cfg.Servers, sc)

	bcfg := backends.BackendConfig{
		"save_workers_size":  3,
		"save_process":       "HeadersParser|Header|Hasher|GotifyBot",
		"log_received_mails": true,
		"primary_mail_host":  smtpConfig.smtpPrimaryHost,
	}
	cfg.BackendConfig = bcfg

	daemon := guerrilla.Daemon{Config: cfg}
	daemon.AddProcessor("GotifyBot", GotifyBotProcessorFactory(gotifyConfig))

	err := daemon.Start()
	return daemon, err
}

func GotifyBotProcessorFactory(
	gotifyConfig *GofityConfig) func() backends.Decorator {
	return func() backends.Decorator {
		// https://github.com/flashmob/go-guerrilla/wiki/Backends,-configuring-and-extending

		return func(p backends.Processor) backends.Processor {
			return backends.ProcessWith(
				func(e *mail.Envelope, task backends.SelectTask) (backends.Result, error) {
					if task == backends.TaskSaveMail {
						err := SendEmailToGotify(e, gotifyConfig)
						if err != nil {
							return backends.NewResult(fmt.Sprintf("554 Error: %s", err)), err
						}
						return p.Process(e, task)
					}
					return p.Process(e, task)
				},
			)
		}
	}
}

func SendEmailToGotify(e *mail.Envelope,
	gotifyConfig *GofityConfig) error {

	title, message := FormatEmail(e,
		gotifyConfig.titleTemplate, gotifyConfig.messageTemplate)

	for _, appToken := range strings.Split(gotifyConfig.gotifyAPIToken, ",") {

		// Apparently the native golang's http client supports
		// http, https and socks5 proxies via HTTP_PROXY/HTTPS_PROXY env vars
		// out of the box.
		//
		// See: https://golang.org/pkg/net/http/#ProxyFromEnvironment
		resp, err := http.PostForm(
			fmt.Sprintf(
				"%smessage?token=%s",
				gotifyConfig.gotifyURL,
				appToken,
			),
			url.Values{
				"title":    {title},
				"message":  {message},
				"priority": {gotifyConfig.gotifyPriority},
			},
		)

		if err != nil {
			return errors.New(SanitizeBotToken(err.Error(), appToken))
		}
		if resp.StatusCode != 200 {
			body, _ := ioutil.ReadAll(resp.Body)
			return errors.New(fmt.Sprintf(
				"Non-200 response from Gotify Server: (%d) %s",
				resp.StatusCode,
				SanitizeBotToken(EscapeMultiLine(body), appToken),
			))
		}
	}
	return nil
}

func FormatEmail(e *mail.Envelope,
	titleTemplate string, messageTemplate string) (string, string) {

	reader := e.NewReader()
	env, err := enmime.ReadEnvelope(reader)
	if err != nil {
		return fmt.Sprintf("smtp-gotify: Could not parse email"),
			fmt.Sprintf("%s\n\nError occurred during email parsing: %s", e, err)
	}
	text := env.Text
	if text == "" {
		text = e.Data.String()
	}
	r := strings.NewReplacer(
		"\\n", "\n",
		"{from}", e.MailFrom.String(),
		"{to}", MapAddresses(e.RcptTo),
		"{subject}", env.GetHeader("subject"),
		"{body}", text,
	)
	return r.Replace(titleTemplate), r.Replace(messageTemplate)
}

func MapAddresses(a []mail.Address) string {
	s := []string{}
	for _, aa := range a {
		s = append(s, aa.String())
	}
	return strings.Join(s, ", ")
}

func EscapeMultiLine(b []byte) string {
	// Apparently errors returned by smtp must not contain newlines,
	// otherwise the data after the first newline is not getting
	// to the parsed message.
	s := string(b)
	s = strings.Replace(s, "\r", "\\r", -1)
	s = strings.Replace(s, "\n", "\\n", -1)
	return s
}

func SanitizeBotToken(s string, botToken string) string {
	return strings.Replace(s, botToken, "***", -1)
}

func sigHandler(d guerrilla.Daemon) {
	signalChannel := make(chan os.Signal, 1)

	signal.Notify(signalChannel,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGINT,
		syscall.SIGKILL,
		os.Kill,
	)
	for range signalChannel {
		d.Log().Infof("Shutdown signal caught")
		go func() {
			select {
			// exit if graceful shutdown not finished in 60 sec.
			case <-time.After(time.Second * 60):
				d.Log().Error("graceful shutdown timed out")
				os.Exit(1)
			}
		}()
		d.Shutdown()
		d.Log().Infof("Shutdown completed, exiting.")
		return
	}
}
