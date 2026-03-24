package main

import (
	"log"
	"net/http"
	"os"

	"github.com/robfig/cron/v3"

	"github.com/mallard/multilegaware/runner"
	"github.com/mallard/multilegaware/telegram"
	"github.com/mallard/multilegaware/tfl"
)

// mustEnv returns the value of the named environment variable or terminates
// the process with a descriptive message if the variable is unset or empty.
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("level=fatal msg=\"required environment variable not set\" var=%q", key)
	}
	return v
}

func main() {
	botToken     := mustEnv("TELEGRAM_BOT_TOKEN")
	chatID       := mustEnv("TELEGRAM_CHAT_ID")
	tflAppKey    := mustEnv("TFL_APP_KEY")
	origin       := mustEnv("ORIGIN")
	destination  := mustEnv("DESTINATION")
	morningCron  := mustEnv("MORNING_CRON")
	afternoonCron := mustEnv("AFTERNOON_CRON")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r := &runner.Runner{
		TfL:      tfl.NewClient(tflAppKey),
		Telegram: telegram.NewClient(botToken),
		ChatID:   chatID,
	}

	// WithSeconds() enables the 6-field cron format (second minute hour dom month dow).
	c := cron.New(cron.WithSeconds())

	if _, err := c.AddFunc(morningCron, func() {
		log.Printf("level=info msg=\"morning run triggered\" from=%q to=%q", origin, destination)
		r.Run(origin, destination)
	}); err != nil {
		log.Fatalf("level=fatal msg=\"invalid MORNING_CRON expression\" expr=%q error=%q", morningCron, err)
	}

	if _, err := c.AddFunc(afternoonCron, func() {
		log.Printf("level=info msg=\"afternoon run triggered\" from=%q to=%q", destination, origin)
		r.Run(destination, origin)
	}); err != nil {
		log.Fatalf("level=fatal msg=\"invalid AFTERNOON_CRON expression\" expr=%q error=%q", afternoonCron, err)
	}

	c.Start()
	log.Printf("level=info msg=\"cron scheduler started\" morning=%q afternoon=%q", morningCron, afternoonCron)
	defer c.Stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}` + "\n"))
	})

	addr := ":" + port
	log.Printf("level=info msg=\"starting server\" addr=%q", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("level=fatal msg=\"server error\" error=%q", err)
	}
}
