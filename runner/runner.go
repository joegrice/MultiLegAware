package runner

import (
	"context"
	"log"
	"time"

	"github.com/mallard/multilegaware/telegram"
	"github.com/mallard/multilegaware/tfl"
)

const maxJourneys = 3

// Runner executes a single journey lookup and dispatches results to Telegram.
type Runner struct {
	TfL      *tfl.Client
	Telegram *telegram.Client
	ChatID   string
}

// Run fetches journeys from → to and sends results sequentially to Telegram.
// All errors are logged; Run never returns an error because it is invoked from
// the cron scheduler where there is no caller to surface errors to.
func (r *Runner) Run(from, to string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()

	journeys, err := r.TfL.GetJourneys(ctx, from, to, maxJourneys)
	if err != nil {
		log.Printf("level=error from=%q to=%q error=%q", from, to, err)
		return
	}

	log.Printf("level=info from=%q to=%q journeys_found=%d tfl_duration_ms=%d",
		from, to, len(journeys), time.Since(start).Milliseconds())

	if len(journeys) == 0 {
		if err := r.Telegram.SendNoJourneysMessage(ctx, r.ChatID, from, to); err != nil {
			log.Printf("level=error msg=\"telegram send failed\" error=%q", err)
		}
		return
	}

	for i, j := range journeys {
		msg := telegram.FormatJourney(i+1, j)
		if err := r.Telegram.SendMessage(ctx, r.ChatID, msg); err != nil {
			log.Printf("level=error msg=\"telegram send failed\" journey_index=%d error=%q", i+1, err)
			break
		}
	}
}
