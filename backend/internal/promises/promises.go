package promises

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/responses"
)

const ExtractorVersion = "ptp-extractor-v1"
const ProfileVersion = "ptp-profile-v1"

type Extraction struct {
	Intent           string    `json:"intent"`
	PromisedFor      time.Time `json:"promised_for"`
	Confidence       float64   `json:"confidence"`
	ExtractorVersion string    `json:"extractor_version"`
	ExtractedAt      time.Time `json:"extraction_timestamp"`
}

var isoDate = regexp.MustCompile(`\b(20\d{2}-\d{2}-\d{2})(?:[ T](\d{2}:\d{2}))?\b`)
var clockTime = regexp.MustCompile(`\b(\d{1,2})(?::(\d{2}))?\s*(am|pm)?\b`)

func Extract(text string, explicit *time.Time, now time.Time, location *time.Location) (Extraction, error) {
	if location == nil {
		location = time.UTC
	}
	now = now.In(location)
	if explicit != nil {
		promised := explicit.In(location)
		if !promised.After(now) {
			return Extraction{}, errors.New("promised time must be in the future")
		}
		return Extraction{Intent: "PROMISE_TO_PAY", PromisedFor: promised, Confidence: .99, ExtractorVersion: ExtractorVersion, ExtractedAt: now}, nil
	}
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return Extraction{}, errors.New("promise text or promised_for is required")
	}
	var promised time.Time
	confidence := 0.0
	if match := isoDate.FindStringSubmatch(lower); match != nil {
		hour, minute := 10, 0
		if match[2] != "" {
			parsed, _ := time.Parse("15:04", match[2])
			hour, minute = parsed.Hour(), parsed.Minute()
		}
		date, err := time.ParseInLocation("2006-01-02", match[1], location)
		if err != nil {
			return Extraction{}, fmt.Errorf("invalid promised date: %w", err)
		}
		promised = time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, location)
		confidence = .95
	} else if strings.Contains(lower, "tomorrow") {
		hour, minute := parseClock(lower, 10, 0)
		next := now.AddDate(0, 0, 1)
		promised = time.Date(next.Year(), next.Month(), next.Day(), hour, minute, 0, 0, location)
		confidence = .88
	} else {
		weekdays := map[string]time.Weekday{"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday, "thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday}
		for name, weekday := range weekdays {
			if strings.Contains(lower, name) {
				days := (int(weekday) - int(now.Weekday()) + 7) % 7
				if days == 0 {
					days = 7
				}
				target := now.AddDate(0, 0, days)
				hour, minute := parseClock(lower, 10, 0)
				promised = time.Date(target.Year(), target.Month(), target.Day(), hour, minute, 0, 0, location)
				confidence = .82
				break
			}
		}
	}
	if promised.IsZero() {
		return Extraction{}, errors.New("ambiguous promised date")
	}
	if !promised.After(now) {
		return Extraction{}, errors.New("promised time must be in the future")
	}
	return Extraction{Intent: "PROMISE_TO_PAY", PromisedFor: promised, Confidence: confidence, ExtractorVersion: ExtractorVersion, ExtractedAt: now}, nil
}

func parseClock(text string, fallbackHour, fallbackMinute int) (int, int) {
	match := clockTime.FindStringSubmatch(text)
	if match == nil {
		return fallbackHour, fallbackMinute
	}
	var hour, minute int
	_, _ = fmt.Sscanf(match[1], "%d", &hour)
	if match[2] != "" {
		_, _ = fmt.Sscanf(match[2], "%d", &minute)
	}
	if match[3] == "pm" && hour < 12 {
		hour += 12
	}
	if match[3] == "am" && hour == 12 {
		hour = 0
	}
	if hour > 23 || minute > 59 {
		return fallbackHour, fallbackMinute
	}
	return hour, minute
}

type CreateInput struct {
	CaseID              domain.ID       `json:"-"`
	Text                string          `json:"text"`
	PromisedFor         *time.Time      `json:"promised_for,omitempty"`
	PromisedAmountMinor *int64          `json:"promised_amount_minor,omitempty"`
	Source              json.RawMessage `json:"source"`
	SourceResponseID    *domain.ID      `json:"source_response_id,omitempty"`
	CorrelationID       string          `json:"correlation_id"`
	Timezone            string          `json:"timezone"`
}

type Store interface {
	CreatePromise(context.Context, domain.PromiseToPay, string) (domain.PromiseToPay, bool, error)
	GetPromise(context.Context, domain.ID) (domain.PromiseToPay, error)
	ListPromises(context.Context, domain.ID) ([]domain.PromiseToPay, error)
	CancelPromise(context.Context, domain.ID, string, time.Time) (domain.PromiseToPay, error)
	ClaimDuePromise(context.Context, string, time.Time, time.Duration) (domain.PromiseToPay, error)
	ResolveDuePromise(context.Context, domain.ID, time.Time) (domain.PromiseToPay, error)
}
type Reassessor interface {
	Reassess(context.Context, domain.ID) error
}
type Service struct {
	store             Store
	reassessor        Reassessor
	now               func() time.Time
	minimumConfidence float64
}

func NewService(store Store, reassessor Reassessor) *Service {
	return &Service{store: store, reassessor: reassessor, now: time.Now, minimumConfidence: .75}
}

var ErrNoDuePromise = errors.New("no due promise check")

func (s *Service) Create(ctx context.Context, input CreateInput) (domain.PromiseToPay, bool, error) {
	location, err := time.LoadLocation(input.Timezone)
	if err != nil {
		if input.Timezone == "" {
			location = time.UTC
		} else {
			return domain.PromiseToPay{}, false, errors.New("invalid IANA timezone")
		}
	}
	extraction, err := Extract(input.Text, input.PromisedFor, s.now().UTC(), location)
	if err != nil {
		return domain.PromiseToPay{}, false, err
	}
	if extraction.Confidence < s.minimumConfidence {
		return domain.PromiseToPay{}, false, errors.New("promise interpretation confidence below threshold")
	}
	if input.PromisedAmountMinor != nil && *input.PromisedAmountMinor <= 0 {
		return domain.PromiseToPay{}, false, errors.New("promised amount must be positive")
	}
	if input.CorrelationID == "" {
		input.CorrelationID = id.New()
	}
	if len(input.Source) == 0 {
		input.Source = json.RawMessage(`{}`)
	}
	promise := domain.PromiseToPay{ID: domain.ID(id.New()), CaseID: input.CaseID, Status: "ACTIVE", DueAt: extraction.PromisedFor.UTC(), Confidence: extraction.Confidence, Source: input.Source, CreatedAt: extraction.ExtractedAt.UTC(), PromisedAmountMinor: input.PromisedAmountMinor, ExtractorVersion: extraction.ExtractorVersion, ExtractionTimestamp: &extraction.ExtractedAt, SourceResponseID: input.SourceResponseID}
	return s.store.CreatePromise(ctx, promise, input.CorrelationID)
}
func (s *Service) Get(ctx context.Context, id domain.ID) (domain.PromiseToPay, error) {
	return s.store.GetPromise(ctx, id)
}
func (s *Service) List(ctx context.Context, caseID domain.ID) ([]domain.PromiseToPay, error) {
	return s.store.ListPromises(ctx, caseID)
}
func (s *Service) Cancel(ctx context.Context, promiseID domain.ID, correlationID string) (domain.PromiseToPay, error) {
	if correlationID == "" {
		correlationID = id.New()
	}
	return s.store.CancelPromise(ctx, promiseID, correlationID, s.now().UTC())
}
func (s *Service) RunDueCheck(ctx context.Context, workerID string) error {
	promise, err := s.store.ClaimDuePromise(ctx, workerID, s.now().UTC(), 2*time.Minute)
	if err != nil {
		return err
	}
	resolved, err := s.store.ResolveDuePromise(ctx, promise.ID, s.now().UTC())
	if err != nil {
		return err
	}
	if resolved.Status == "BROKEN" && s.reassessor != nil {
		return s.reassessor.Reassess(ctx, resolved.CaseID)
	}
	return nil
}

// CreateFromResponse is deliberately deterministic and idempotent through the
// source_response_id unique key. It accepts legacy due_at as well as the richer
// promised_for/text payload used by the public promise API.
func (s *Service) CreateFromResponse(ctx context.Context, response responses.Response) error {
	var body struct {
		Text                string     `json:"text"`
		PromisedFor         *time.Time `json:"promised_for"`
		DueAt               *time.Time `json:"due_at"`
		PromisedAmountMinor *int64     `json:"promised_amount_minor"`
		Timezone            string     `json:"timezone"`
	}
	if err := json.Unmarshal(response.Payload, &body); err != nil {
		return errors.New("invalid promise response payload")
	}
	if body.PromisedFor == nil {
		body.PromisedFor = body.DueAt
	}
	_, _, err := s.Create(ctx, CreateInput{CaseID: response.CaseID, Text: body.Text, PromisedFor: body.PromisedFor, PromisedAmountMinor: body.PromisedAmountMinor, Source: response.Payload, SourceResponseID: &response.ID, CorrelationID: response.CorrelationID, Timezone: body.Timezone})
	return err
}
