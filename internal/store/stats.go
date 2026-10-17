package store

import (
	"context"
	"database/sql"
	"sort"
	"strings"
)

// DecadeLifespan is the age at death of people born in one decade.
type DecadeLifespan struct {
	Decade  int     `json:"decade"`
	Count   int     `json:"count"`
	Average float64 `json:"average"`
	Min     int     `json:"min"`
	Max     int     `json:"max"`
}

// Average is a mean over Count values; Average is 0 when Count is 0.
type Average struct {
	Count   int     `json:"count"`
	Average float64 `json:"average"`
}

// DecadeMarriage is the age at marriage of people born in one decade.
type DecadeMarriage struct {
	Decade     int     `json:"decade"`
	FirstMen   Average `json:"firstMen"`
	FirstWomen Average `json:"firstWomen"`
	// Later are second and further marriages of both sexes.
	Later Average `json:"later"`
}

// NameCount is how often a name occurs.
type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// DecadeNames are the most common first given names of one birth decade.
type DecadeNames struct {
	Decade int         `json:"decade"`
	Men    []NameCount `json:"men"`
	Women  []NameCount `json:"women"`
}

// Stats are figures about the whole tree.
type Stats struct {
	Persons  int `json:"persons"`
	Families int `json:"families"`
	Sources  int `json:"sources"`
	// Deceased with a known age at death.
	WithLifespan int              `json:"withLifespan"`
	Lifespans    []DecadeLifespan `json:"lifespans"`
	Marriages    []DecadeMarriage `json:"marriages"`
	// ChildrenHistogram[n] is the number of families with n children; the
	// last bucket holds that many or more.
	ChildrenHistogram []int         `json:"childrenHistogram"`
	ChildrenAverage   float64       `json:"childrenAverage"`
	Surnames          []NameCount   `json:"surnames"`
	GivenNames        []NameCount   `json:"givenNames"`
	GivenNameTrends   []DecadeNames `json:"givenNameTrends"`
}

const (
	maxSurnames       = 60
	maxGivenNames     = 20
	namesPerDecade    = 3
	childrenBuckets   = 11
	plausibleMinAge   = 10
	plausibleMaxAge   = 110
	plausibleMaxDeath = 120
)

// ageBetween is whole years from one YYYYMMDD key to a later one; an
// unknown month or day counts as not yet reached.
func ageBetween(from, to int) int {
	y := to/10000 - from/10000
	fm, fd, tm, td := from/100%100, from%100, to/100%100, to%100
	if tm < fm || (tm == fm && td < fd) {
		y--
	}
	return y
}

func decadeOf(key int) int { return key / 10000 / 10 * 10 }

// Stats computes the statistics of the tree. Disproven facts and dates
// that are only bounded (BEF, AFT) are left out.
func (s *Store) Stats(ctx context.Context, a Actor) (Stats, error) {
	st := Stats{Lifespans: []DecadeLifespan{}, Marriages: []DecadeMarriage{}, Surnames: []NameCount{}, GivenNames: []NameCount{},
		GivenNameTrends: []DecadeNames{}, ChildrenHistogram: make([]int, childrenBuckets)}
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM sources WHERE tree_id = ?`, a.TreeID).Scan(&st.Sources); err != nil {
		return st, err
	}
	type person struct {
		given, surname, sex string
		birth, death        int
	}
	persons := map[int64]*person{}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT p.id, p.given_names, p.surname, p.sex,
			coalesce((SELECT e.date_sort FROM events e WHERE e.person_id = p.id AND e.type IN ('BIRT', 'CHR', 'BAPM')
				AND e.status <> 'disproven' AND e.date_sort > 0 AND e.date_qualifier NOT IN ('BEF', 'AFT', 'TO')
				ORDER BY e.type <> 'BIRT', e.date_sort LIMIT 1), 0),
			coalesce((SELECT e.date_sort FROM events e WHERE e.person_id = p.id AND e.type IN ('DEAT', 'BURI', 'CREM')
				AND e.status <> 'disproven' AND e.date_sort > 0 AND e.date_qualifier NOT IN ('BEF', 'AFT', 'FROM')
				ORDER BY e.type <> 'DEAT', e.date_sort LIMIT 1), 0)
		FROM persons p WHERE p.tree_id = ?`, a.TreeID)
	if err != nil {
		return st, err
	}
	for rows.Next() {
		var id int64
		var p person
		if err := rows.Scan(&id, &p.given, &p.surname, &p.sex, &p.birth, &p.death); err != nil {
			rows.Close()
			return st, err
		}
		persons[id] = &p
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return st, err
	}
	st.Persons = len(persons)

	type agg struct {
		n, sum, min, max int
	}
	life := map[int]*agg{}
	for _, p := range persons {
		if p.birth == 0 || p.death == 0 {
			continue
		}
		age := ageBetween(p.birth, p.death)
		if age < 0 || age > plausibleMaxDeath {
			continue
		}
		d := decadeOf(p.birth)
		g := life[d]
		if g == nil {
			g = &agg{min: age, max: age}
			life[d] = g
		}
		g.n++
		g.sum += age
		g.min, g.max = min(g.min, age), max(g.max, age)
		st.WithLifespan++
	}
	for d, g := range life {
		st.Lifespans = append(st.Lifespans, DecadeLifespan{Decade: d, Count: g.n, Average: round1(float64(g.sum) / float64(g.n)), Min: g.min, Max: g.max})
	}
	sort.Slice(st.Lifespans, func(i, j int) bool { return st.Lifespans[i].Decade < st.Lifespans[j].Decade })

	rows, err = s.DB.QueryContext(ctx, `
		SELECT f.partner1_id, f.partner2_id, e.date_sort FROM families f JOIN events e ON e.family_id = f.id
		WHERE f.tree_id = ? AND e.type = 'MARR' AND e.status <> 'disproven' AND e.date_sort > 0
			AND e.date_qualifier NOT IN ('BEF', 'AFT')
		ORDER BY e.date_sort`, a.TreeID)
	if err != nil {
		return st, err
	}
	married := map[int64][]int{}
	for rows.Next() {
		var p1, p2 sql.NullInt64
		var key int
		if err := rows.Scan(&p1, &p2, &key); err != nil {
			rows.Close()
			return st, err
		}
		for _, p := range []sql.NullInt64{p1, p2} {
			if p.Valid {
				married[p.Int64] = append(married[p.Int64], key)
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return st, err
	}
	type marr struct{ men, women, later agg }
	byDecade := map[int]*marr{}
	add := func(g *agg, age int) { g.n++; g.sum += age }
	for id, keys := range married {
		p := persons[id]
		if p == nil || p.birth == 0 {
			continue
		}
		d := decadeOf(p.birth)
		m := byDecade[d]
		if m == nil {
			m = &marr{}
			byDecade[d] = m
		}
		for i, k := range keys {
			age := ageBetween(p.birth, k)
			if age < plausibleMinAge || age > plausibleMaxAge {
				continue
			}
			switch {
			case i > 0:
				add(&m.later, age)
			case p.sex == "M":
				add(&m.men, age)
			case p.sex == "F":
				add(&m.women, age)
			}
		}
	}
	avg := func(g agg) Average {
		if g.n == 0 {
			return Average{}
		}
		return Average{Count: g.n, Average: round1(float64(g.sum) / float64(g.n))}
	}
	for d, m := range byDecade {
		if m.men.n+m.women.n+m.later.n == 0 {
			continue
		}
		st.Marriages = append(st.Marriages, DecadeMarriage{Decade: d, FirstMen: avg(m.men), FirstWomen: avg(m.women), Later: avg(m.later)})
	}
	sort.Slice(st.Marriages, func(i, j int) bool { return st.Marriages[i].Decade < st.Marriages[j].Decade })

	// A family without known partners is a placeholder for unknown
	// parents, not a family whose children could be counted.
	fams, err := s.familyLinks(ctx, a)
	if err != nil {
		return st, err
	}
	total := 0
	for _, f := range fams {
		if f.p1 == 0 && f.p2 == 0 {
			continue
		}
		st.Families++
		n := len(f.children)
		total += n
		st.ChildrenHistogram[min(n, childrenBuckets-1)]++
	}
	if st.Families > 0 {
		st.ChildrenAverage = round1(float64(total) / float64(st.Families))
	}

	surnames := map[string]int{}
	given := map[string]int{}
	trends := map[int]map[string]map[string]int{}
	for _, p := range persons {
		if sn := strings.TrimSpace(p.surname); sn != "" {
			surnames[sn]++
		}
		first := strings.Fields(p.given)
		if len(first) == 0 {
			continue
		}
		name := first[0]
		given[name]++
		if p.birth != 0 && (p.sex == "M" || p.sex == "F") {
			d := decadeOf(p.birth)
			if trends[d] == nil {
				trends[d] = map[string]map[string]int{"M": {}, "F": {}}
			}
			trends[d][p.sex][name]++
		}
	}
	st.Surnames = topNames(surnames, maxSurnames)
	st.GivenNames = topNames(given, maxGivenNames)
	for d, bySex := range trends {
		st.GivenNameTrends = append(st.GivenNameTrends, DecadeNames{Decade: d, Men: topNames(bySex["M"], namesPerDecade), Women: topNames(bySex["F"], namesPerDecade)})
	}
	sort.Slice(st.GivenNameTrends, func(i, j int) bool { return st.GivenNameTrends[i].Decade < st.GivenNameTrends[j].Decade })
	return st, nil
}

// topNames are the n most frequent names, ties in alphabetical order.
func topNames(counts map[string]int, n int) []NameCount {
	out := make([]NameCount, 0, len(counts))
	for name, c := range counts {
		out = append(out, NameCount{name, c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }
