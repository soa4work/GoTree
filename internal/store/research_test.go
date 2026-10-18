package store

import (
	"errors"
	"testing"
)

func TestResearchTasks(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	anna := mustPerson(t, s, a, "Anna", "Weber")
	src, err := s.CreateSource(ctx, a, SourceInput{Title: "Kirchenbuch Leipzig"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.CreateTask(ctx, a, ResearchTaskInput{Title: "", DueOn: "soon"})
	validationField(t, err, "title")
	_, err = s.CreateTask(ctx, a, ResearchTaskInput{Title: "x", DueOn: "soon"})
	validationField(t, err, "dueOn")
	_, err = s.CreateTask(ctx, a, ResearchTaskInput{Title: "x", Links: []ResearchLinkInput{{EntityType: "person", EntityID: 999}}})
	validationField(t, err, "links")

	task, err := s.CreateTask(ctx, a, ResearchTaskInput{
		Title: "Find Anna's baptism", Priority: "high", DueOn: "2026-11-01",
		Links: []ResearchLinkInput{{EntityType: "person", EntityID: anna.ID}, {EntityType: "source", EntityID: src.ID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "open" || len(task.Links) != 2 || task.Links[0].Label != "Anna Weber" || task.Links[1].Label != "Kirchenbuch Leipzig" {
		t.Errorf("task: %+v", task)
	}
	later, _ := s.CreateTask(ctx, a, ResearchTaskInput{Title: "Later"})

	// Searching in vain is worth recording too.
	entry, err := s.CreateLogEntry(ctx, a, LogEntryInput{
		TaskID: &task.ID, SearchedOn: "2026-10-06", Query: "Baptisms 1850–1855", Location: "Stadtarchiv Leipzig", Result: "not_found",
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.TaskTitle != "Find Anna's baptism" {
		t.Errorf("entry: %+v", entry)
	}
	_, err = s.CreateLogEntry(ctx, a, LogEntryInput{SearchedOn: "yesterday", Query: "x", Result: "maybe"})
	validationField(t, err, "searchedOn")

	mine, err := s.ListTasks(ctx, a, ResearchFilter{PersonID: anna.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].LogCount != 1 {
		t.Errorf("Anna's tasks: %+v", mine)
	}
	log, err := s.ListLog(ctx, a, ResearchFilter{PersonID: anna.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 {
		t.Errorf("a log entry counts for the people of its task: %+v", log)
	}

	done, err := s.UpdateTask(ctx, a, task.ID, ResearchTaskInput{Title: task.Title, Status: "done", Priority: "high"})
	if err != nil {
		t.Fatal(err)
	}
	if done.DoneAt == nil || len(done.Links) != 0 {
		t.Errorf("done: %+v", done)
	}
	active, _ := s.ListTasks(ctx, a, ResearchFilter{Status: "active"})
	if len(active) != 1 || active[0].ID != later.ID {
		t.Errorf("active: %+v", active)
	}

	if err := s.DeleteTask(ctx, a, task.ID); err != nil {
		t.Fatal(err)
	}
	kept, err := s.ListLog(ctx, a, ResearchFilter{})
	if err != nil || len(kept) != 1 || kept[0].TaskID != nil {
		t.Errorf("the log survives its task: %+v %v", kept, err)
	}
}

func TestResearchLinksFollowDeletes(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	anna := mustPerson(t, s, a, "Anna", "")
	task, err := s.CreateTask(ctx, a, ResearchTaskInput{Title: "x", Links: []ResearchLinkInput{{EntityType: "person", EntityID: anna.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePerson(ctx, a, anna.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.getTask(ctx, s.DB, a, task.ID)
	if len(got.Links) != 0 {
		t.Errorf("links to a deleted person: %+v", got.Links)
	}
}

func TestSuggestions(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	old := mustPerson(t, s, a, "Old", "Weber")
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &old.ID, Type: "BIRT", Date: "1800"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &old.ID, Type: "BURI", Date: "1790"}); err != nil {
		t.Fatal(err)
	}

	rep, err := s.Suggestions(ctx, a, old.ID)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]Suggestion{}
	for _, sg := range rep.Suggestions {
		kinds[sg.Kind] = sg
	}
	// Born, but without a source; buried, so the death is covered; and the
	// burial before the birth gives two findings.
	if len(rep.Suggestions) != 3 || kinds["birth_unsourced"].Origin == "" || kinds["finding"].Finding == nil {
		t.Fatalf("suggestions: %+v", rep.Suggestions)
	}

	f := kinds["finding"]
	if _, err := s.CreateTask(ctx, a, ResearchTaskInput{Title: "Check the burial", Origin: f.Origin}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTask(ctx, a, ResearchTaskInput{Title: "Again", Origin: f.Origin}); !errors.Is(err, ErrConflict) {
		t.Errorf("the same suggestion twice: %v", err)
	}
	rep, _ = s.Suggestions(ctx, a, old.ID)
	if len(rep.Suggestions) != 2 || rep.Suggestions[0].Kind != "birth_unsourced" {
		t.Errorf("taken suggestions are left out: %+v", rep.Suggestions)
	}
	checks, _ := s.Checks(ctx, a, old.ID)
	if checks.TaskIDs[f.Origin] == 0 {
		t.Errorf("the check report knows the task: %+v", checks.TaskIDs)
	}

	_, err = s.CreateTask(ctx, a, ResearchTaskInput{Title: "x", Origin: "made up"})
	validationField(t, err, "origin")

	// Deleting the person frees the origin, even if the id comes back.
	if err := s.DeletePerson(ctx, a, old.ID); err != nil {
		t.Fatal(err)
	}
	if origins, _ := s.taskOrigins(ctx, a, ""); len(origins) != 0 {
		t.Errorf("origins of a deleted person: %v", origins)
	}

	nobody := mustPerson(t, s, a, "Nobody", "")
	rep, _ = s.Suggestions(ctx, a, nobody.ID)
	if len(rep.Suggestions) != 1 || rep.Suggestions[0].Kind != "missing_birth" {
		t.Errorf("unknown birth, presumed living: %+v", rep.Suggestions)
	}
	if _, err := s.Suggestions(ctx, a, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("no person: %v", err)
	}
}
