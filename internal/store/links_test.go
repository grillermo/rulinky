package store

import (
	"context"
	"testing"
	"time"
)

func newUser(t *testing.T, st *Store, email string) User {
	t.Helper()
	u, err := st.FindOrCreateUserByEmail(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestCreateAndListLinks(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u := newUser(t, st, "me@example.com")

	l1, job1, err := st.CreateLink(ctx, u.ID, "https://a.com", "first")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Millisecond)
	l2, _, err := st.CreateLink(ctx, u.ID, "https://b.com", "")
	if err != nil {
		t.Fatal(err)
	}

	links, err := st.ListLinks(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 || links[0].ID != l2.ID || links[1].ID != l1.ID {
		t.Fatalf("want newest first, got %+v", links)
	}
	got := links[1]
	if got.URL != "https://a.com" || got.Note == nil || *got.Note != "first" || got.Read != 0 ||
		got.Timestamp != got.UpdatedAt || got.Timestamp < time.Now().Add(-time.Minute).UnixMilli() {
		t.Fatalf("unexpected link %+v", got)
	}
	status, _, found, err := st.JobStatus(ctx, job1)
	if err != nil || !found || status != "queued" {
		t.Fatalf("job: status=%q found=%v err=%v", status, found, err)
	}

	other := newUser(t, st, "other@example.com")
	if links, _ := st.ListLinks(ctx, other.ID); len(links) != 0 {
		t.Fatalf("other user sees %d links", len(links))
	}
}

func TestListLinksLoadsContentOnlyWithoutRawTitle(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u := newUser(t, st, "me@example.com")
	withTitle, _, _ := st.CreateLink(ctx, u.ID, "https://a.com", "")
	withoutTitle, _, _ := st.CreateLink(ctx, u.ID, "https://b.com", "")
	_, _ = st.Pool().Exec(ctx, `UPDATE links SET raw_title = 'T', content = '<p>big</p>' WHERE id = $1`, withTitle.ID)
	_, _ = st.Pool().Exec(ctx, `UPDATE links SET raw_title = '', content = '<title>C</title>' WHERE id = $1`, withoutTitle.ID)

	links, _ := st.ListLinks(ctx, u.ID)
	for _, l := range links {
		switch l.ID {
		case withTitle.ID:
			if l.Content != nil || l.RawTitle == nil || *l.RawTitle != "T" {
				t.Errorf("with raw title: %+v", l)
			}
		case withoutTitle.ID:
			if l.Content == nil || *l.Content != "<title>C</title>" {
				t.Errorf("without raw title: %+v", l)
			}
		}
	}
}

func TestSetLinkRead(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u := newUser(t, st, "me@example.com")
	l, _, _ := st.CreateLink(ctx, u.ID, "https://a.com", "")
	time.Sleep(3 * time.Millisecond)

	other := newUser(t, st, "other@example.com")
	if ok, err := st.SetLinkRead(ctx, other.ID, l.ID, true); ok || err != nil {
		t.Fatalf("other user updated link: ok=%v err=%v", ok, err)
	}
	if ok, err := st.SetLinkRead(ctx, u.ID, l.ID, true); !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	links, _ := st.ListLinks(ctx, u.ID)
	if links[0].Read != 1 || links[0].UpdatedAt <= l.UpdatedAt {
		t.Fatalf("read not set or updated_at not bumped: %+v", links[0])
	}
	_, _ = st.SetLinkRead(ctx, u.ID, l.ID, false)
	links, _ = st.ListLinks(ctx, u.ID)
	if links[0].Read != 0 {
		t.Fatalf("unread not set: %+v", links[0])
	}
}

func TestDeleteLink(t *testing.T) {
	st := OpenTest(t)
	ctx := context.Background()
	u := newUser(t, st, "me@example.com")
	l, jobID, _ := st.CreateLink(ctx, u.ID, "https://a.com", "")

	other := newUser(t, st, "other@example.com")
	if ok, err := st.DeleteLink(ctx, other.ID, l.ID); ok || err != nil {
		t.Fatalf("other user deleted link: ok=%v err=%v", ok, err)
	}
	if ok, err := st.DeleteLink(ctx, u.ID, l.ID); !ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if links, _ := st.ListLinks(ctx, u.ID); len(links) != 0 {
		t.Fatal("link still listed")
	}
	if _, _, found, _ := st.JobStatus(ctx, jobID); found {
		t.Fatal("job not deleted with its link")
	}
	if ok, _ := st.DeleteLink(ctx, u.ID, l.ID); ok {
		t.Fatal("second delete reported success")
	}
}
