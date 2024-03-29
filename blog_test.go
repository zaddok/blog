package blog

import (
	"fmt"
	"testing"
	"time"

	"github.com/zaddok/log"
	"gitlab.com/montebo/security"
)

// TestBlogEntry tests Getting and Searching four blog entries
func TestBlogEntry(t *testing.T) {

	l := log.NewStdoutLogDebug()
	defer l.Close()

	{
		l.Debug("GAE TestBlogEntry")
		am, err, client, context := security.NewGaeAccessManager(projectId, inferLocation(t), time.Now().Location(), l)
		if err != nil {
			t.Fatalf("NewGaeAccessManager() failed: %v", err)
		}
		bm := NewGaeBlogManager(client, context, am)
		testBlogEntry(am, bm, t)
	}

	{
		l.Debug("CQL TestBlogEntry")
		am, cql, err := security.NewCqlAccessManager(TestCqlKeyspace, testCassandraHostname, "", time.Now().Location(), l)
		if err != nil {
			t.Fatalf("NewCqlAccessManager() failed: %v", err)
		}
		if err != nil {
			t.Fatalf("NewCqlBlogManager() failed: %v", err)
		}
		bm, err := NewCqlBlogManager(cql, am, l)
		testBlogEntry(am, bm, t)
	}

}

func testBlogEntry(am security.AccessManager, bm BlogManager, t *testing.T) {

	_, err := am.AddPerson(TestSite, "entry", "manager", "entry_manager@mysite.com", "s1:s2:s3:s4:c1:c2:c3:c4:c5:c6", security.HashPassword("tmp1!aAfo"), "127.0.0.1", nil)
	if err != nil {
		t.Fatalf("AddPerson() failed: %v", err)
	}
	session, _, err := am.Authenticate(TestSite, "entry_manager@mysite.com", "tmp1!aAfo", "127.0.0.1", "", "en-AU", "", "Australia/Melbourne")
	if err != nil {
		t.Fatalf("Authenticate() failed: %v", err)
	}
	if !session.IsAuthenticated() {
		t.Fatalf("Authenticate() authentication for entry_manager@example.com failed.")
	}

	_, err = am.AddPerson(TestSite, "Jane", "Li", "jane.li@example.com", "s1:s2:s3:s4:c1:c2:c3:c4:c5:c6", security.HashPassword("tmp1!aAfo"), "127.0.0.1", nil)
	if err != nil {
		t.Fatalf("AddPerson() failed: %v", err)
	}
	_, err = am.AddPerson(TestSite, "William", "Wang", "william.wang@example.com", "s1:s2:s3:s4:c1:c2:c3:c4:c5:c6", security.HashPassword("tmp1!aAfo"), "127.0.0.1", nil)
	if err != nil {
		t.Fatalf("AddPerson() failed: %v", err)
	}
	_, err = am.AddPerson(TestSite, "Andrew", "Wang", "andrew.wang@example.com", "s1:s2:s3:s4:c1:c2:c3:c4:c5:c6", security.HashPassword("tmp1!aAfo"), "127.0.0.1", nil)
	if err != nil {
		t.Fatalf("AddPerson() failed: %v", err)
	}

	p1, _ := am.GetPersonByEmail(TestSite, "jane.li@example.com", session)
	p2, _ := am.GetPersonByEmail(TestSite, "william.wang@example.com", session)
	p3, _ := am.GetPersonByEmail(TestSite, "andrew.wang@example.com", session)

	entry0 := bm.NewEntry()
	entry0.SetTitle("A Title")
	entry0.SetDescription("Simple description")
	entry0.SetCover("cover.jpg")
	entry0.SetThumbnail("thumbnail.jpg")
	entry0.SetText("Does _this_ blog entry need some *text*?")
	entry0.SetDate(*StringToDatePointer("2000/1/1"))
	entry0.SetAuthor(p1)
	entry0.SetTags([]string{"a", "b"})
	err = bm.AddEntry(entry0, session)
	if err != nil {
		t.Fatalf("AddEntry() failed unexpectedly: %v", err)
		return
	}

	entry1 := bm.NewEntry()
	entry1.SetTitle("First entry")
	entry1.SetDescription("Location 1")
	entry1.SetText("Some *text* for this blog.")
	entry1.SetDate(*StringToDatePointer("2000/1/2"))
	entry1.SetTags([]string{"c", "d"})
	entry1.SetAuthor(p2)
	err = bm.AddEntry(entry1, session)
	if err != nil {
		t.Fatalf("AddEntry() failed unexpectedly: %v", err)
		return
	}

	entry2 := bm.NewEntry()
	entry2.SetTitle("Second entry Luke 1:1-2")
	entry2.SetDescription("Location 2")
	entry2.SetCover("coverX.jpg")
	entry2.SetThumbnail("thumbnailX.jpg")
	entry2.SetText("Sample _text_ for blog.")
	entry2.SetDate(*StringToDatePointer("2100/1/1"))
	entry2.SetAuthor(p3)
	err = bm.AddEntry(entry2, session)
	if err != nil {
		t.Fatalf("AddEntry() failed unexpectedly: %v", err)
		return
	}

	{
		ev, err := bm.GetEntry(entry2.Uuid(), session)
		if err != nil {
			t.Fatalf("GetEntry() failed unexpectedly: %v", err)
			return
		}
		if ev.Title() != entry2.Title() {
			t.Fatalf("GetEntry() Incorrect title, returned %v", ev.Title())
		}
		if ev.Cover() != entry2.Cover() {
			t.Fatalf("GetEntry() Incorrect cover, returned %v", ev.Cover())
		}
		if ev.Description() != entry2.Description() {
			t.Fatalf("GetEntrys() Incorrect description, returned %v", ev.Description())
		}
		if ev.Date().Unix() != entry2.Date().Unix() {
			t.Fatalf("GetEntrys() Incorrect end time, returned %v", ev.Date())
		}
		if ev.Author() == nil {
			t.Fatalf("GetAuthor() returned nil")
		}
		if ev.Author().FirstName() != "Andrew" {
			t.Fatalf("GetAuthor() returned %s, should return 'Andrew'", ev.Author().FirstName())
		}
	}

	{
		ev, err := bm.GetEntryBySlug("a-title", session)
		if err != nil {
			t.Fatalf("GetEntryBySlug() failed unexpectedly: %v", err)
			return
		}
		if ev == nil {
			t.Fatalf("GetEntryBySlug() returned nil")
		}
		if ev.Title() != "A Title" {
			t.Fatalf("GetEntryBySlug() Incorrect title, returned %v", ev.Title())
		}
	}

	{
		ev, err := bm.GetEntryBySlug("a-title-archie", session)
		if err != nil {
			t.Fatalf("GetEntryBySlug() failed unexpectedly: %v", err)
			return
		}
		if ev != nil {
			t.Fatalf("GetEntryBySlug() should return nil")
		}
	}

	{
		eci, _ := bm.GetRecentEntries(10, session)
		if len(eci) != 2 {
			t.Fatalf("We expect 2 entries here, not %d", len(eci))
		}

		ev, err := bm.GetEntry(entry1.Uuid(), session)
		if err != nil {
			t.Fatalf("GetEntry() failed unexpectedly: %v", err)
		}
		if ev.Title() != entry1.Title() {
			t.Fatalf("GetEntry() Incorrect name, returned %v", ev.Title())
		}
		if ev.Uuid() != entry1.Uuid() {
			t.Fatalf("GetEntry() Incorrect uuid, returned %v", ev.Uuid())
		}
		ev.SetTitle("Updated title")
		ev.SetCover("coverV.jpg")
		ev.SetText("t2")
		ev.SetThumbnail("thumbnailV.jpg")
		ev.SetTags([]string{"z", "x", "y"})
		err = bm.UpdateEntry(ev, session)
		if err != nil {
			t.Fatalf("%s UpdateEntry() failed unexpectedly: %v", am.Type(), err)
			return
		}
		eci, _ = bm.GetRecentEntries(10, session)
		if len(eci) != 2 {
			t.Fatalf("We expect 2 entries here, not %d", len(eci))
		}
		ev, err = bm.GetEntry(entry1.Uuid(), session)
		if err != nil {
			t.Fatalf("GetEntry() failed unexpectedly: %v", err)
			return
		}
		if ev.Title() != "Updated title" {
			t.Fatalf("UpdateEntry() Incorrect title, returned %v", ev.Title())
		}
		if ev.Text() != "t2" {
			t.Fatalf("UpdateEntry() Incorrect title, returned %v", ev.Text())
		}
		if ev.Cover() != "coverV.jpg" {
			t.Fatalf("UpdateEntry() Incorrect cover, returned %v", ev.Cover())
		}
		if ev.Thumbnail() != "thumbnailV.jpg" {
			t.Fatalf("%s UpdateEntry() Incorrect thumbnail, returned %v", am.Type(), ev.Thumbnail())
		}
	}

	{
		entrys, err := bm.GetRecentEntries(10, session)
		if err != nil {
			t.Fatalf("GetRecentEntries() failed unexpectedly: %v", err)
			return
		}
		if entrys == nil {
			t.Fatalf("GetRecentEntries() Did not return two entrys")
		}
		if len(entrys) != 2 {
			t.Fatalf("GetRecentEntries() Did not return two entrys. Returned %d", len(entrys))
		}
		for _, e := range entrys {
			fmt.Printf("Found %v\n", e.Date())
		}
	}

	{
		entrys, err := bm.GetFutureEntries(session)
		if err != nil {
			t.Fatalf("GetFutureEntries() failed unexpectedly: %v", err)
			return
		}
		if entrys == nil {
			t.Fatalf("GetFutureEntries() Did not return one entry")
		}
		if len(entrys) != 1 {
			t.Fatalf("GetFutureEntries() Did not return one entrys. Returned %d", len(entrys))
		}
		if entrys[0].Date().Year() != 2100 {
			t.Fatalf("GetFutureEntries() Did not return correct future entry. Returned  entry on %v, expected %v", entrys[0].Date(), entry2.Uuid())
		}
		if entrys[0].Uuid() != entry2.Uuid() {
			t.Fatalf("GetFutureEntries() Did not return correct future entry. %s != %s", entrys[0].Uuid(), entry2.Uuid())
		}

	}
}
