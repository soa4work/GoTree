package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/praetorianer777/gotree/internal/gendate"
)

// CalendarFeed is a secret subscription URL for birthdays, wedding
// anniversaries and remembrance days.
type CalendarFeed struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
	// IncludeLiving adds living people's birthdays and living couples'
	// anniversaries; without it only the deceased appear.
	IncludeLiving bool    `json:"includeLiving"`
	RevokedAt     *string `json:"revokedAt"`
	LastUsedAt    *string `json:"lastUsedAt"`
	CreatedAt     string  `json:"createdAt"`
	// Token is only returned when the feed is created.
	Token string `json:"token,omitempty"`
}

// CalendarFeedInput creates a feed.
type CalendarFeedInput struct {
	Label         string `json:"label"`
	IncludeLiving bool   `json:"includeLiving"`
}

// CreateCalendarFeed makes a new feed. Only the owner may create one,
// since the URL hands out data without a login.
func (s *Store) CreateCalendarFeed(ctx context.Context, a Actor, in CalendarFeedInput) (CalendarFeed, error) {
	if a.Role != RoleOwner {
		return CalendarFeed{}, ErrOwnerOnly
	}
	in.Label = strings.TrimSpace(in.Label)
	var v validator
	v.check(in.Label != "" && utf8.RuneCountInString(in.Label) <= 120, "label", "give the feed a name of at most 120 characters")
	if err := v.err(); err != nil {
		return CalendarFeed{}, err
	}
	token, err := newToken()
	if err != nil {
		return CalendarFeed{}, err
	}
	var feed CalendarFeed
	err = s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO calendar_feeds (tree_id, token_hash, label, include_living, created_at, created_by)
			VALUES (?, ?, ?, ?, ?, ?)`, a.TreeID, hashToken(token), in.Label, in.IncludeLiving, s.now(), nullID(a.UserID))
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		feeds, err := s.calendarFeeds(ctx, tx, a, `id = ?`, id)
		if err != nil {
			return err
		}
		feed = feeds[0]
		return s.logChange(ctx, tx, a, "calendar_feed", id, "create", nil, feed)
	})
	feed.Token = token
	return feed, err
}

// ListCalendarFeeds lists the tree's feeds, newest first.
func (s *Store) ListCalendarFeeds(ctx context.Context, a Actor) ([]CalendarFeed, error) {
	if a.Role != RoleOwner {
		return nil, ErrOwnerOnly
	}
	return s.calendarFeeds(ctx, s.DB, a, `1 = 1`)
}

// RevokeCalendarFeed stops a feed; subscribed calendars get 404 from then on.
func (s *Store) RevokeCalendarFeed(ctx context.Context, a Actor, id int64) error {
	if a.Role != RoleOwner {
		return ErrOwnerOnly
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.calendarFeeds(ctx, tx, a, `id = ?`, id)
		if err != nil {
			return err
		}
		if len(before) == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `UPDATE calendar_feeds SET revoked_at = coalesce(revoked_at, ?) WHERE id = ? AND tree_id = ?`,
			s.now(), id, a.TreeID); err != nil {
			return err
		}
		after, err := s.calendarFeeds(ctx, tx, a, `id = ?`, id)
		if err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "calendar_feed", id, "update", before[0], after[0])
	})
}

func (s *Store) calendarFeeds(ctx context.Context, q queryer, a Actor, where string, args ...any) ([]CalendarFeed, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, label, include_living, revoked_at, last_used_at, created_at FROM calendar_feeds
		WHERE tree_id = ? AND `+where+` ORDER BY created_at DESC, id DESC`, append([]any{a.TreeID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	feeds := []CalendarFeed{}
	for rows.Next() {
		var f CalendarFeed
		var revoked, used sql.NullString
		if err := rows.Scan(&f.ID, &f.Label, &f.IncludeLiving, &revoked, &used, &f.CreatedAt); err != nil {
			return nil, err
		}
		f.RevokedAt, f.LastUsedAt = strPtr(revoked), strPtr(used)
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

// CalendarEntry is one yearly recurring day.
type CalendarEntry struct {
	// UID is stable across requests, so calendar apps update instead of
	// duplicating entries.
	UID  string
	Kind string // BIRT, MARR or DEAT
	// Year, Month and Day are the Gregorian date of the original event.
	Year, Month, Day int
	// Names is empty for a person without a name.
	Names []string
}

// Calendar is the content of a feed.
type Calendar struct {
	TreeName string
	Entries  []CalendarEntry
}

// CalendarByToken resolves a feed token and returns its entries. Unknown
// and revoked feeds give ErrNotFound.
func (s *Store) CalendarByToken(ctx context.Context, token string) (Calendar, error) {
	if token == "" {
		return Calendar{}, ErrNotFound
	}
	var id int64
	var a Actor
	var cal Calendar
	var includeLiving bool
	err := s.DB.QueryRowContext(ctx, `
		SELECT f.id, f.tree_id, t.name, f.include_living FROM calendar_feeds f JOIN trees t ON t.id = f.tree_id
		WHERE f.token_hash = ? AND f.revoked_at IS NULL`, hashToken(token)).Scan(&id, &a.TreeID, &cal.TreeName, &includeLiving)
	if errors.Is(err, sql.ErrNoRows) {
		return Calendar{}, ErrNotFound
	}
	if err != nil {
		return Calendar{}, err
	}
	a.Role = RoleViewer
	now := s.Now().UTC()
	if _, err := s.DB.ExecContext(ctx, `UPDATE calendar_feeds SET last_used_at = ? WHERE id = ? AND (last_used_at IS NULL OR last_used_at < ?)`,
		now.Format(time.RFC3339Nano), id, now.Add(-time.Minute).Format(time.RFC3339Nano)); err != nil {
		return Calendar{}, err
	}

	living, err := s.livingAll(ctx, a)
	if err != nil {
		return Calendar{}, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT e.id, e.type, e.date_sort, e.person_id, f.partner1_id, f.partner2_id
		FROM events e LEFT JOIN families f ON f.id = e.family_id
		WHERE e.tree_id = ? AND e.type IN ('BIRT', 'MARR', 'DEAT') AND e.status <> 'disproven'
			AND e.date_qualifier = '' AND e.date_sort > 0 AND e.date_sort % 100 <> 0
		ORDER BY e.date_sort % 10000, e.id`, a.TreeID)
	if err != nil {
		return Calendar{}, err
	}
	defer rows.Close()
	type pending struct {
		entry CalendarEntry
		ids   []int64
	}
	var found []pending
	var ids []int64
	for rows.Next() {
		var eventID, key int64
		var kind string
		var person, p1, p2 sql.NullInt64
		if err := rows.Scan(&eventID, &kind, &key, &person, &p1, &p2); err != nil {
			return Calendar{}, err
		}
		var people []int64
		for _, p := range []sql.NullInt64{person, p1, p2} {
			if p.Valid {
				people = append(people, p.Int64)
			}
		}
		if len(people) == 0 {
			continue
		}
		show := true
		for _, pid := range people {
			if living[pid] && !includeLiving {
				show = false
			}
			// A living person's death day cannot exist; a living partner's
			// anniversary is covered by includeLiving above.
			if kind == "DEAT" && living[pid] {
				show = false
			}
		}
		if !show {
			continue
		}
		y, m, d := gendate.SplitKey(int(key))
		e := CalendarEntry{
			UID:  "gotree-" + strconv.FormatInt(a.TreeID, 10) + "-" + strconv.FormatInt(eventID, 10),
			Kind: kind, Year: y, Month: m, Day: d,
		}
		found = append(found, pending{e, people})
		ids = append(ids, people...)
	}
	if err := rows.Err(); err != nil {
		return Calendar{}, err
	}
	rows.Close()
	refs, err := s.personRefsChunked(ctx, a, ids)
	if err != nil {
		return Calendar{}, err
	}
	for _, p := range found {
		for _, id := range p.ids {
			// An empty name stays empty: the API puts "Unknown" in the
			// language of the feed.
			name := strings.TrimSpace(refs[id].GivenNames + " " + refs[id].Surname)
			p.entry.Names = append(p.entry.Names, name)
		}
		cal.Entries = append(cal.Entries, p.entry)
	}
	return cal, nil
}
