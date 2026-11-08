// Package daemon schedules independent daily optimizations and five-minute
// quote checks; all completed evaluations and alerts persist across restarts.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/marcgauthier/Crypto/internal/findings"
)

type ScanFunc func(context.Context, string, time.Time) ([]findings.Rule, error)
type CheckFunc func(context.Context, findings.Rule, time.Time) (string, error)
type CheckFactory func(context.Context, time.Time) (CheckFunc, func() error, error)

type Service struct {
	Store     *findings.Store
	Pairs     []string
	Scan      ScanFunc
	Checks    CheckFactory
	Logger    *log.Logger
	failureAt map[string]time.Time
}

func NextFiveMinute(t time.Time) time.Time {
	return t.UTC().Truncate(5 * time.Minute).Add(5 * time.Minute)
}
func NextMidnightUTC(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day()+1, 0, 0, 0, 0, time.UTC)
}

func (s *Service) notice(format string, args ...any) {
	if s.Logger != nil {
		s.Logger.Printf(format, args...)
	}
}

// Tick first catches up on a missing UTC evaluation, then checks both legs.
// A 5-minute check never causes a second successful daily optimization.
func (s *Service) Tick(ctx context.Context, now time.Time) error {
	now = now.UTC()
	day := now.Format("2006-01-02")
	var problems []error
	if s.failureAt == nil {
		s.failureAt = map[string]time.Time{}
	}
	for _, pair := range s.Pairs {
		done, err := s.Store.Evaluated(ctx, pair, day)
		if err != nil {
			return err
		}
		if done || s.Scan == nil {
			continue
		}
		if last := s.failureAt[pair]; !last.IsZero() && now.Sub(last) < 30*time.Minute {
			continue
		}
		rules, err := s.Scan(ctx, pair, now)
		if err == nil {
			err = s.Store.PublishRules(ctx, pair, day, rules, now)
		}
		if err != nil {
			s.failureAt[pair] = now
			problems = append(problems, fmt.Errorf("daily evaluation %s: %w", pair, err))
			s.notice("daily evaluation failed for %s: %v", pair, err)
		} else {
			delete(s.failureAt, pair)
			s.notice("daily evaluation saved for %s (%s)", pair, day)
		}
	}
	if s.Checks == nil {
		return errors.Join(problems...)
	}
	check, closeFn, err := s.Checks(ctx, now)
	if err != nil {
		return errors.Join(append(problems, err)...)
	}
	if closeFn != nil {
		defer func() {
			if err := closeFn(); err != nil {
				s.notice("metrics close: %v", err)
			}
		}()
	}
	for _, pair := range s.Pairs {
		rules, err := s.Store.Rules(ctx, pair)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		for _, r := range rules {
			if r.EvaluationDay != day {
				_ = s.Store.NoteStatus(ctx, r, now, "evaluation_overdue", fmt.Errorf("latest rules are from %s", r.EvaluationDay))
				continue // fail closed rather than trade on stale parameters
			}
			action, err := check(ctx, r, now)
			if err != nil {
				problems = append(problems, fmt.Errorf("check %s: %w", r.Symbol, err))
				s.notice("check %s: %v", r.Symbol, err)
			}
			if action != "" {
				s.notice("PAPER %s %s (%s) at %s", action, r.Symbol, pair, now.Format(time.RFC3339))
			}
		}
	}
	return errors.Join(problems...)
}

// Run checks once at startup (catch-up after downtime), then ticks on exact
// UTC 5-minute boundaries. The midnight boundary is one of these ticks.
func (s *Service) Run(ctx context.Context) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := s.Tick(ctx, time.Now().UTC()); err != nil {
			s.notice("tick: %v", err)
		}
		wait := time.Until(NextFiveMinute(time.Now()))
		if wait < time.Millisecond {
			wait = time.Millisecond
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}
	}
}

// ParsePairs normalizes and validates configurable independent bullish/inverse pairs.
func ParsePairs(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.ToUpper(strings.TrimSpace(p))
		leg := strings.Split(p, ":")
		if len(leg) != 2 || leg[0] == "" || leg[1] == "" || leg[0] == leg[1] {
			return nil, fmt.Errorf("invalid pair %q", p)
		}
		for _, ticker := range leg {
			for _, c := range ticker {
				if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-') {
					return nil, fmt.Errorf("invalid symbol %q", ticker)
				}
			}
		}
		if seen[p] {
			return nil, fmt.Errorf("duplicate pair %q", p)
		}
		seen[p] = true
		out = append(out, p)
	}
	return out, nil
}
