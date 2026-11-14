package chatview

import (
	"cmp"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/ui"
	"github.com/nsgo-dev/nsgo/ui/chat"
	agio "github.com/oneo-me/nsgo-agio"
	"github.com/oneo-me/nsgo-agio/chatview/locales"
	"github.com/oneo-me/nsgo-agio/quota"
)

// quotaRows are an account's limits as rows: a bar of what is left for
// each window and an amount for each balance.
func quotaRows(msgs locales.Messages, q *agio.Quota, now time.Time) []chat.QuotaRow {
	if q.Usage == nil {
		return nil
	}
	var rows []chat.QuotaRow
	for _, w := range q.Usage.Windows {
		left := min(max(100-w.Percent, 0), 100)
		row := chat.QuotaRow{Label: quotaWindowLabel(msgs, w), Value: msgs.QuotaLeft(fmt.Sprintf("%.0f", left)), Fraction: left / 100}
		if !w.ResetsAt.IsZero() {
			row.Detail = quotaReset(msgs, w.ResetsAt, now)
		}
		rows = append(rows, row)
	}
	for _, b := range q.Usage.Balances {
		rows = append(rows, chat.QuotaRow{Label: msgs.QuotaBalance(), Value: formatBalance(b), Fraction: -1})
	}
	return rows
}

// quotaPanel is an account's limits with the time to its next read, a
// refresh and a link to its usage page.
func quotaPanel(m *Model, msgs locales.Messages, p quota.Provider) chat.Quota {
	find := func() *agio.Quota { return providerQuota(m.State(), p) }
	q := find()
	o := chat.Quota{ID: string(p), Title: p.Name(), Leading: leading(quotaIcon(p)), Note: quotaNote(msgs, q),
		RefreshLabel: msgs.QuotaRefresh(p.Name()),
		Loading:      live(m, func() bool { q := find(); return q.Usage == nil && q.Err == "" }),
		Refreshing:   live(m, func() bool { return find().Checking }),
		Rows:         live(m, func() []chat.QuotaRow { return quotaRows(msgs, find(), m.now()) }),
		Countdown:    live(m, func() string { return quotaCountdown(msgs, find(), m.now()) }),
		OnRefresh:    func() { m.Do(func(b agio.Backend) error { b.RefreshQuota(p); return nil }, nil) }}
	if q.Usage != nil && q.Usage.Plan != "" {
		o.Title += " · " + q.Usage.Plan
	}
	if page := p.UsagePage(); page != "" {
		o.OpenLabel = msgs.QuotaOpenPage(p.Name())
		o.OnOpen = func() { m.cx.OpenURL(page, nil) }
	}
	return o
}

// providerQuota is an account's limits, empty before they are read.
func providerQuota(s agio.State, p quota.Provider) *agio.Quota {
	for _, q := range s.Quotas {
		if q.Provider == p {
			return &q
		}
	}
	return &agio.Quota{Provider: p}
}

// quotaFailed reports whether an account has no limits to show because
// reading them failed.
func quotaFailed(q *agio.Quota) int {
	if q.Usage == nil && q.Err != "" {
		return 1
	}
	return 0
}

// quotaStatus is why an account has no limit to show, in a word or two;
// its panel's note explains.
func quotaStatus(msgs locales.Messages, q *agio.Quota) string {
	switch {
	case q.Expired:
		return msgs.QuotaExpiredShort()
	case q.Err != "":
		return msgs.QuotaUnavailable()
	}
	return msgs.QuotaRefreshing()
}

// quotaNote explains a failed read. Earlier limits stay meaningful while
// the provider rate-limits reads, so that says nothing.
func quotaNote(msgs locales.Messages, q *agio.Quota) string {
	switch {
	case q.Err == "", q.Usage != nil && q.Status == http.StatusTooManyRequests:
		return ""
	case q.Expired:
		return msgs.QuotaExpired()
	case q.Status == http.StatusTooManyRequests:
		return msgs.QuotaRateLimited()
	case q.Status == http.StatusUnauthorized || q.Status == http.StatusForbidden:
		if q.Message != "" {
			return q.Message
		}
		return msgs.QuotaSignin()
	}
	return msgs.QuotaFailed(q.Err)
}

func quotaWindowLabel(msgs locales.Messages, w quota.Window) string {
	var label string
	switch {
	case w.Minutes <= 0:
		label = msgs.QuotaLimit()
	case w.Minutes == 7*24*60:
		label = msgs.QuotaWeekly()
	case w.Minutes < 24*60:
		label = msgs.QuotaHours((w.Minutes + 59) / 60)
	default:
		label = msgs.QuotaDays((w.Minutes + 24*60 - 1) / (24 * 60))
	}
	if w.Scope != "" {
		label += " · " + w.Scope
	}
	return label
}

// quotaReset uses the two largest whole units remaining, dropping a zero
// second unit, with seconds rounded up.
func quotaReset(msgs locales.Messages, at, now time.Time) string {
	d := at.Sub(now)
	switch {
	case d <= 0:
		return msgs.QuotaResetsSoon()
	case d < time.Minute:
		return msgs.QuotaResetsSeconds(int((d + time.Second - 1) / time.Second))
	case d < time.Hour:
		return msgs.QuotaResetsMinutes(int(d / time.Minute))
	case d < 24*time.Hour:
		hours, minutes := int(d/time.Hour), int(d%time.Hour/time.Minute)
		if minutes == 0 {
			return msgs.QuotaResetsHours(hours)
		}
		return msgs.QuotaResetsHoursMinutes(hours, minutes)
	}
	days, hours := int(d/(24*time.Hour)), int(d%(24*time.Hour)/time.Hour)
	if hours == 0 {
		return msgs.QuotaResetsDays(days)
	}
	return msgs.QuotaResetsDaysHours(days, hours)
}

// quotaCountdown is the time to the next read as MM:SS, with hours for
// longer backoffs, rounded up so it never shows zero early.
func quotaCountdown(msgs locales.Messages, q *agio.Quota, now time.Time) string {
	if q.Checking || q.Next.IsZero() {
		return msgs.QuotaRefreshing()
	}
	d := max(q.Next.Sub(now), 0)
	seconds := int64((d + time.Second - 1) / time.Second)
	if seconds >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
	}
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func formatBalance(b quota.Balance) string {
	switch b.Currency {
	case "CNY":
		return "¥" + b.Amount
	case "USD":
		return "$" + b.Amount
	}
	return b.Amount + " " + b.Currency
}

// quotaSummary is the one limit the sidebar shows for an account: the
// shortest window limiting all of its models, unless one is exhausted,
// then the one that blocks longest, else a balance. Windows scoped to some
// models count only when no other applies. It uses the latest limits read,
// never assuming a window refilled once its reset passed.
func quotaSummary(msgs locales.Messages, q *agio.Quota, now time.Time) (chat.QuotaRow, bool) {
	if q == nil || q.Usage == nil {
		return chat.QuotaRow{}, false
	}
	u := q.Usage
	before := func(a, b quota.Window) bool {
		if (a.Scope == "") != (b.Scope == "") {
			return a.Scope == ""
		}
		return quotaWindowBefore(a, b)
	}
	index := -1
	for i, w := range u.Windows {
		if index < 0 || before(w, u.Windows[index]) {
			index = i
		}
	}
	if index < 0 {
		if len(u.Balances) == 0 {
			return chat.QuotaRow{}, false
		}
		index = len(u.Windows)
	}
	return quotaRows(msgs, q, now)[index], true
}

func quotaWindowBefore(a, b quota.Window) bool {
	if aOut, bOut := a.Percent >= 100, b.Percent >= 100; aOut != bOut {
		return aOut
	} else if aOut {
		return quotaResetsLater(a, b)
	}
	if a.Minutes != b.Minutes {
		return a.Minutes > 0 && (b.Minutes <= 0 || a.Minutes < b.Minutes)
	}
	return quotaResetsLater(b, a)
}

// quotaResetsLater orders unknown reset times last.
func quotaResetsLater(a, b quota.Window) bool {
	if a.ResetsAt.IsZero() || b.ResetsAt.IsZero() {
		return a.ResetsAt.IsZero() && !b.ResetsAt.IsZero()
	}
	return a.ResetsAt.After(b.ResetsAt)
}

// quotas end the sidebar with each account signed in on two lines: its
// mark, name and what is left of its limit, then a bar of it, opening all
// of its limits. A balance has no bar, nor an account whose limits could
// not be read, which says why instead.
func (a App) quotas(cx *core.Scope) core.Node {
	m := a.Model
	// The panels' titles and notes are fixed, so they rebuild the lines.
	key := func() string {
		msgs := locales.For(cx)
		var b strings.Builder
		fmt.Fprint(&b, i18n.Language(cx))
		for _, q := range m.State().Quotas {
			r, ok := quotaSummary(msgs, &q, time.Time{})
			fmt.Fprint(&b, "\x01", quotaFailed(&q))
			fmt.Fprint(&b, "\x00", q.Provider, "\x01", quotaNote(msgs, &q), "\x01", ok && r.Fraction >= 0)
			if q.Usage != nil {
				fmt.Fprint(&b, "\x01", q.Usage.Plan)
			}
		}
		return b.String()
	}
	return region(m, key, func(cx *core.Scope) core.Node {
		msgs := locales.For(cx)
		var lines []core.Node
		// Accounts whose limits cannot be read, such as an expired
		// sign-in, go last.
		quotas := slices.Clone(m.State().Quotas)
		slices.SortStableFunc(quotas, func(a, b agio.Quota) int {
			return cmp.Compare(quotaFailed(&a), quotaFailed(&b))
		})
		for _, q := range quotas {
			lines = append(lines, quotaLine(cx, m, msgs, q.Provider))
		}
		if len(lines) == 0 {
			return core.Group(nil)
		}
		return core.View{TestID: "quotas", Style: core.Style{Padding: core.Edges{Top: 8, Right: 12, Left: 12}, Gap: 2, FlexShrink: 0, MinWidth: core.Pt(0)}, Children: lines}
	})
}

// quotaLine is an account's line in the sidebar. Its bar warns from 20%
// left and alerts from 5%, as the limits' panel does.
func quotaLine(cx *core.Scope, m *Model, msgs locales.Messages, p quota.Provider) core.Node {
	colors := ui.ColorsOf(cx)
	summary := live(m, func() chat.QuotaRow { r, _ := quotaSummary(msgs, providerQuota(m.State(), p), m.now()); return r })
	value := ui.Textf(live(m, func() string {
		q := providerQuota(m.State(), p)
		if r, ok := quotaSummary(msgs, q, m.now()); ok {
			return r.Value
		}
		return quotaStatus(msgs, q)
	}), 12, ui.Regular, colors.Muted)
	value.Style.FlexShrink = 0
	children := []core.Node{row(6, mark(cx, quotaIcon(p), 13), fillLine(p.Name(), 12.5, colors.Text), value)}
	if r, ok := quotaSummary(msgs, providerQuota(m.State(), p), time.Time{}); ok && r.Fraction >= 0 {
		children = append(children, core.View{Style: core.Style{Height: core.Pt(4), CornerRadius: 2, Overflow: core.Hidden}, StyleFunc: ui.Filled(colors.Track),
			Children: []core.Node{core.View{Style: core.Style{Height: core.Pct(100), CornerRadius: 2}, StyleFunc: func() core.Style {
				f := summary().Fraction
				fill := colors.Accent()
				switch {
				case f <= 0.05:
					fill = colors.Danger()
				case f <= 0.2:
					fill = colors.Warn()
				}
				return core.Style{Width: core.Pct(float32(100 * min(max(f, 0), 1))), Background: fill}
			}}}})
	}
	open := core.NewSignal(cx, false)
	return base.Popover{Open: open.Get, OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) }, Placement: core.PlaceTop, Align: core.AlignStart, Offset: 4, Margin: 8,
		Label: msgs.QuotaDetails(p.Name()), TestID: "quota-" + string(p),
		Style: core.Style{Padding: core.Symmetric(6, 8), CornerRadius: 6, Gap: 6, MinWidth: core.Pt(0)},
		StyleFunc: func() core.Style {
			s := core.Style{Hover: &core.Style{Background: ui.Alpha(colors.Text(), .05)}, Focus: ui.FocusRing(cx, -2)}
			if open.Get() {
				s.Background = ui.Alpha(colors.Text(), .05)
			}
			return s
		},
		Trigger:    column(6, children...),
		PopupStyle: core.Style{Width: core.Pt(320), MaxWidth: core.Pct(100), Padding: core.All(14), Gap: 14},
		PopupStyleFunc: func() core.Style {
			return ui.FinOf(cx).Menu().Over(core.Style{CornerRadius: ui.RadiusCard, BorderWidth: 1, Color: colors.Text()})
		},
		Children: []core.Node{quotaPanel(m, msgs, p)}}
}
