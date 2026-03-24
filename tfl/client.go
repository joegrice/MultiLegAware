package tfl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const baseURL = "https://api.tfl.gov.uk/Journey/JourneyResults"

// Client calls the TfL Unified API.
type Client struct {
	appKey     string
	httpClient *http.Client
}

func NewClient(appKey string) *Client {
	return &Client{
		appKey: appKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// --- TfL response structs (only fields we use) ---

type JourneyResponse struct {
	Journeys []Journey `json:"journeys"`
}

type Journey struct {
	Duration int   `json:"duration"`
	Legs     []Leg `json:"legs"`
}

type Leg struct {
	Duration      int         `json:"duration"`
	DepartureTime string      `json:"departureTime"`
	ArrivalTime   string      `json:"arrivalTime"`
	Mode          Mode        `json:"mode"`
	Instruction   Instruction `json:"instruction"`
}

type Mode struct {
	Name string `json:"name"`
}

type Instruction struct {
	Summary string `json:"summary"`
}

// GetJourneys fetches journey options from TfL and returns up to limit results.
func (c *Client) GetJourneys(ctx context.Context, from, to string, limit int) ([]Journey, error) {
	endpoint := fmt.Sprintf("%s/%s/to/%s", baseURL, url.PathEscape(from), url.PathEscape(to))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("User-Agent", "MultiLegAware/1.0")

	q := req.URL.Query()
	q.Set("nationalSearch", "true")
	if c.appKey != "" {
		q.Set("app_key", c.appKey)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling TfL API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TfL API returned status %d", resp.StatusCode)
	}

	var result JourneyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding TfL response: %w", err)
	}

	journeys := result.Journeys
	if len(journeys) > limit {
		journeys = journeys[:limit]
	}
	return journeys, nil
}
