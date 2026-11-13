package host

import (
	"context"
	"errors"
	"net/http"
	"time"

	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/quota"
)

// Automatic reads occur at most once per interval, because the usage
// endpoints rate-limit eager clients; Claude's does so most, so it waits
// longest.
const (
	quotaInterval       = 5 * time.Minute
	claudeQuotaInterval = 15 * time.Minute
)

func quotaIntervalFor(p quota.Provider) time.Duration {
	if p == quota.Claude {
		return claudeQuotaInterval
	}
	return quotaInterval
}

// quotaState is one account's latest limits. A failed read keeps the
// previous usage; ErrNoCredentials hides the account. A provider asking to
// back off is not read again on its own before retry.
type quotaState struct {
	usage    *quota.Usage
	err      error
	checking bool
	checked  time.Time
	retry    time.Time
}

// next is when the account is read again: after its interval, or soon
// after a window resets within it, but not before a backoff ends.
func (q *quotaState) next(p quota.Provider) time.Time {
	at := q.checked.Add(quotaIntervalFor(p))
	if r := q.usage.NextReset(q.checked); !r.IsZero() && r.Add(quota.ResetDelay).Before(at) {
		at = r.Add(quota.ResetDelay)
	}
	if q.checked.IsZero() || q.retry.After(at) {
		return q.retry
	}
	return at
}

// watchQuotas reads the accounts that are due each second, once the login
// environment, which may relocate credentials, is known.
func (h *Host) watchQuotas() {
	if h.opts.ReadQuota == nil {
		return
	}
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-h.done:
				return
			case <-tick.C:
				// Most ticks read nothing, and tell no one.
				h.mu.Lock()
				started := false
				for _, p := range quota.Providers {
					started = h.readQuota(p, false) || started
				}
				h.mu.Unlock()
				if started {
					h.changed()
				}
			}
		}
	}()
}

// RefreshQuota implements agio.Backend: it reads the account now, unless
// a read is under way.
func (h *Host) RefreshQuota(p quota.Provider) {
	h.post(func() { h.readQuota(p, true) })
}

// readQuota starts reading an account off the lock when due, or when
// forced, and reports whether it did.
func (h *Host) readQuota(p quota.Provider, force bool) bool {
	if h.opts.ReadQuota == nil || h.env == nil || h.closed {
		return false
	}
	q := h.quotas[p]
	if q == nil {
		q = &quotaState{}
		h.quotas[p] = q
	}
	now := h.opts.Now()
	if q.checking || !force && now.Before(q.next(p)) {
		return false
	}
	q.checking, q.checked = true, now
	read, env := h.opts.ReadQuota, h.env
	go func() {
		usage, err := read(context.Background(), env, p)
		h.post(func() {
			q.checking, q.checked, q.err, q.retry = false, h.opts.Now(), err, time.Time{}
			// Being rate-limited without a Retry-After backs off for twice
			// the interval.
			var status *quota.StatusError
			if errors.As(err, &status) {
				wait := status.RetryAfter
				if status.Status == http.StatusTooManyRequests {
					wait = max(wait, 2*quotaIntervalFor(p))
				}
				if wait > 0 {
					q.retry = q.checked.Add(wait)
				}
			}
			switch {
			case err == nil:
				q.usage = &usage
			case errors.Is(err, quota.ErrNoCredentials):
				q.usage = nil
			}
		})
	}()
	return true
}

// quotaStates are the accounts read once, in order, but those not signed
// in.
func (h *Host) quotaStates() []agio.Quota {
	var out []agio.Quota
	for _, p := range quota.Providers {
		q := h.quotas[p]
		if q == nil || errors.Is(q.err, quota.ErrNoCredentials) || q.usage == nil && q.err == nil {
			continue
		}
		s := agio.Quota{Provider: p, Usage: q.usage, Checking: q.checking, Checked: q.checked, Next: q.next(p)}
		if q.err != nil {
			s.Err = q.err.Error()
			var status *quota.StatusError
			switch {
			case errors.Is(q.err, quota.ErrExpired):
				s.Expired = true
			case errors.As(q.err, &status):
				s.Status, s.Message = status.Status, status.Message
			}
		}
		out = append(out, s)
	}
	return out
}
