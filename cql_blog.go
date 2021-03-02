package blog

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/bluele/gcache"
	"github.com/gocql/gocql"
	"github.com/zaddok/log"
	"gitlab.com/montebo/security"
)

func NewCqlBlogManager(cql *gocql.Session, am security.AccessManager, log log.Log) *CqlBlogManager {
	s := &CqlBlogManager{
		cql:        cql,
		am:         am,
		log:        log,
		entryCache: gcache.New(200).LRU().Expiration(time.Second * 3600).Build(),
		slugCache:  gcache.New(200).LRU().Expiration(time.Second * 3600).Build(),
	}
	return s
}

type CqlBlogManager struct {
	cql        *gocql.Session
	log        log.Log
	am         security.AccessManager
	entryCache gcache.Cache
	slugCache  gcache.Cache
}

func (bm *CqlBlogManager) NewEntry() Entry {
	return &GaeEntry{}
}

func (bm *CqlBlogManager) AccessManager() security.AccessManager {
	return bm.am
}

func (bm *CqlBlogManager) GetEntry(uuid string, session security.Session) (Entry, error) {
	var entry GaeEntry

	rows := bm.cql.Query("select title, slug, description, tags, date, created, updated, author, text from blog_entry where uuid=?",
		uuid).Iter()
	if !rows.Scan(&entry.title, &entry.slug, &entry.description, &entry.tags, &entry.date, &entry.created, &entry.updated, &entry.authorUuid, &entry.text) {
		return nil, rows.Close()
	}

	// Found result
	err := rows.Close()
	if err != nil {
		return nil, err
	}

	if entry.authorUuid != "" {
		entry.author, err = bm.am.GetPersonCached(entry.authorUuid, session)
		if err != nil {
			return nil, err
		}
	}

	return &entry, nil
}

func (bm *CqlBlogManager) GetRecentEntries(limit int, session security.Session) ([]Entry, error) {
	var items []Entry
	var err error
	now := time.Now()

	rows := bm.cql.Query("select title, slug, description, tags, date, created, updated, author, text from blog_entry").Iter()
	entry := &GaeEntry{}
	for rows.Scan(&entry.title, &entry.slug, &entry.description, &entry.tags, &entry.date, &entry.created, &entry.updated, &entry.authorUuid, &entry.text) {
		if entry.date.Before(now) {
			if entry.authorUuid != "" {
				entry.author, err = bm.am.GetPersonCached(entry.authorUuid, session)
				if err != nil {
					return nil, err
				}
			}
			items = append(items, entry)
		}
	}

	err = rows.Close()
	if err != nil {
		return nil, err
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Date() != nil && items[j].Date() != nil {
			return items[j].Date().Before(*items[i].Date())
		}
		if items[j].Date() != nil && items[i].Created() != nil {
			return items[j].Date().Before(*items[i].Created())
		}
		if items[i].Date() != nil && items[j].Created() != nil {
			return items[j].Created().Before(*items[i].Date())
		}
		return items[j].Created().Before(*items[i].Created())
	})

	if len(items) > limit {
		return items[0:limit], nil
	}
	return items[:], nil
}

func (bm *CqlBlogManager) GetEntriesByAuthor(personUuid string, session security.Session) ([]Entry, error) {
	var items []Entry
	var err error
	now := time.Now()

	rows := bm.cql.Query("select title, slug, description, tags, date, created, updated, author, text from blog_entry where author=?", personUuid).Iter()
	entry := &GaeEntry{}
	for rows.Scan(&entry.title, &entry.slug, &entry.description, &entry.tags, &entry.date, &entry.created, &entry.updated, &entry.authorUuid, &entry.text) {
		if entry.date.After(now) {
			if entry.authorUuid != "" {
				entry.author, err = bm.am.GetPersonCached(entry.authorUuid, session)
				if err != nil {
					return nil, err
				}
			}
			items = append(items, entry)
		}
	}

	err = rows.Close()
	if err != nil {
		return nil, err
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Date() != nil && items[j].Date() != nil {
			return items[j].Date().Before(*items[i].Date())
		}
		if items[j].Date() != nil && items[i].Created() != nil {
			return items[j].Date().Before(*items[i].Created())
		}
		if items[i].Date() != nil && items[j].Created() != nil {
			return items[j].Created().Before(*items[i].Date())
		}
		return items[j].Created().Before(*items[i].Created())
	})

	return items, nil

}

// SearchEntries returns all entries matching a specified keyword. Take care
// to ensure users have permission to view each search entry. Search results
// may include future unpublished blog articles.
func (bm *CqlBlogManager) SearchEntries(query string, session security.Session) ([]Entry, error) {
	var items []Entry
	var err error

	fields := strings.Fields(strings.ToLower(query))
	sort.Slice(fields, func(i, j int) bool {
		return len(fields[j]) < len(fields[i])
	})

	if len(fields) == 0 {
		return nil, nil
	}

	rows := bm.cql.Query("select title, slug, description, tags, date, created, updated, author, text from blog_entry where search_tags=?", fields).Iter()
	entry := &GaeEntry{}
	for rows.Scan(&entry.title, &entry.slug, &entry.description, &entry.tags, &entry.date, &entry.created, &entry.updated, &entry.authorUuid, &entry.text) {
		if entry.authorUuid != "" {
			entry.author, err = bm.am.GetPersonCached(entry.authorUuid, session)
			if err != nil {
				return nil, err
			}
		}
		items = append(items, entry)
	}

	err = rows.Close()
	if err != nil {
		return nil, err
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Date() != nil && items[j].Date() != nil {
			return items[j].Date().Before(*items[i].Date())
		}
		if items[j].Date() != nil && items[i].Created() != nil {
			return items[j].Date().Before(*items[i].Created())
		}
		if items[i].Date() != nil && items[j].Created() != nil {
			return items[j].Created().Before(*items[i].Date())
		}
		return items[j].Created().Before(*items[i].Created())
	})

	return items, nil

}
func (bm *CqlBlogManager) GetFutureEntries(session security.Session) ([]Entry, error) {
	var items []Entry
	var err error
	now := time.Now()

	rows := bm.cql.Query("select title, slug, description, tags, date, created, updated, author, text from blog_entry").Iter()
	entry := &GaeEntry{}
	for rows.Scan(&entry.title, &entry.slug, &entry.description, &entry.tags, &entry.date, &entry.created, &entry.updated, &entry.authorUuid, &entry.text) {
		if entry.date.After(now) {
			if entry.authorUuid != "" {
				entry.author, err = bm.am.GetPersonCached(entry.authorUuid, session)
				if err != nil {
					return nil, err
				}
			}
			items = append(items, entry)
		}
	}

	err = rows.Close()
	if err != nil {
		return nil, err
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Date() != nil && items[j].Date() != nil {
			return items[j].Date().Before(*items[i].Date())
		}
		if items[j].Date() != nil && items[i].Created() != nil {
			return items[j].Date().Before(*items[i].Created())
		}
		if items[i].Date() != nil && items[j].Created() != nil {
			return items[j].Created().Before(*items[i].Date())
		}
		return items[j].Created().Before(*items[i].Created())
	})

	return items, nil
}

func (bm *CqlBlogManager) GetEntryBySlug(slug string, session security.Session) (Entry, error) {
	var entry GaeEntry

	rows := bm.cql.Query("select title, slug, description, tags, date, created, updated, author, text from blog_entry where slug=?",
		slug).Iter()
	if !rows.Scan(&entry.title, &entry.slug, &entry.description, &entry.tags, &entry.date, &entry.created, &entry.updated, &entry.authorUuid, &entry.text) {
		return nil, rows.Close()
	}

	// Found result
	err := rows.Close()
	if err != nil {
		return nil, err
	}

	if entry.authorUuid != "" {
		entry.author, err = bm.am.GetPersonCached(entry.authorUuid, session)
		if err != nil {
			return nil, err
		}
	}

	return &entry, nil
}

func (bm *CqlBlogManager) GetEntryCached(uuid string, session security.Session) (Entry, error) {

	if uuid == "" {
		return nil, nil
	}

	v, _ := bm.entryCache.Get(uuid)
	if v != nil {
		entry := v.(Entry)
		return entry, nil
	}

	entry, err := bm.GetEntry(uuid, session)
	if err != nil {
		return nil, err
	}
	bm.entryCache.Set(entry.Uuid(), entry)
	bm.slugCache.Set(entry.Slug(), entry)

	return entry, nil
}

func (bm *CqlBlogManager) GetEntryBySlugCached(slug string, session security.Session) (Entry, error) {

	if slug == "" {
		return nil, nil
	}

	v, _ := bm.slugCache.Get(slug)
	if v != nil {
		entry := v.(Entry)
		return entry, nil
	}

	entry, err := bm.GetEntryBySlug(slug, session)
	if err != nil {
		return nil, err
	}
	bm.entryCache.Set(entry.Uuid(), entry)
	bm.slugCache.Set(entry.Slug(), entry)

	return entry, nil
}

func (bm *CqlBlogManager) AddEntry(entry Entry, session security.Session) error {
	if session == nil || !session.IsAuthenticated() {
		return &security.ErrUnauthenticated{session}
	}

	if entry.Title() == "" {
		return errors.New("Entry must have a title")
	}
	if entry.Text() == "" {
		return errors.New("Entry must contain text")
	}

	bulk := &security.GaeEntityAuditLogCollection{}
	bulk.SetEntityUuidPersonUuid(entry.Uuid(), session.PersonUuid(), session.DisplayName())

	if entry.Title() != "" {
		bulk.AddItem("Title", "", entry.Title())
	}

	if entry.Description() != "" {
		bulk.AddItem("Description", "", entry.Description())
	}

	if entry.Slug() != "" {
		bulk.AddItem("Slug", "", entry.Slug())
	}

	if entry.Date() != nil {
		bulk.AddDateItem("Date", nil, entry.Date())
	}

	if entry.Text() != "" {
		bulk.AddItem("Text", "", entry.Text())
	}

	if entry.Author() != nil {
		bulk.AddItem("Author", "", entry.Author().Uuid())
	}

	// TODO: Technically should be in a transaction
	if err := bm.am.AddEntityChangeLog(bulk, session); err != nil {
		return err
	}

	rows := bm.cql.Query(
		"update blog_entry set title=?, slug=?, description=?, tags=?, date=?, created=?, updated=?, author=?, text=?, search_tags=? where uuid=?",
		entry.Title(),
		entry.Slug(),
		entry.Description(),
		entry.Tags(),
		entry.Date(),
		entry.Created(),
		entry.Updated(),
		entry.Author(),
		entry.Text(),
		entry.SearchTags(),
		entry.Uuid()).Iter()
	err := rows.Close()
	if err != nil {
		return err
	}

	bm.entryCache.Set(entry.Uuid(), entry)
	bm.slugCache.Set(entry.Slug(), entry)

	return nil
}

func (bm *CqlBlogManager) UpdateEntry(entry Entry, session security.Session) error {
	if session == nil || !session.IsAuthenticated() {
		return &security.ErrUnauthenticated{session}
	}

	if entry.Title() == "" {
		return errors.New("Entry must have a title")
	}
	if entry.Text() == "" {
		return errors.New("Entry must contain text")
	}

	var current GaeEntry
	rows := bm.cql.Query("select title, slug, description, tags, date, created, updated, author, text from blog_entry where uuid=?",
		entry.Uuid()).Iter()
	if !rows.Scan(
		&current.title,
		&current.slug,
		&current.description,
		&current.tags,
		&current.date,
		&current.created,
		&current.updated,
		&current.authorUuid,
		&current.text) {
		err := rows.Close()
		if err == nil {
			return errors.New("No entry has this uuid")
		}
		return err
	}
	err := rows.Close()
	if err != nil {
		return err
	}

	bulk := &security.GaeEntityAuditLogCollection{}
	bulk.SetEntityUuidPersonUuid(entry.Uuid(), session.PersonUuid(), session.DisplayName())

	if !security.MatchingDate(entry.Date(), current.Date()) {
		bulk.AddDateItem("Date", current.Date(), entry.Date())
		current.SetDate(*entry.Date())
	}

	if entry.Title() != current.Title() {
		bulk.AddItem("Title", current.Title(), entry.Title())
		current.SetTitle(entry.Title())
	}

	if entry.Description() != current.Description() {
		bulk.AddItem("Description", current.Description(), entry.Description())
		current.SetDescription(entry.Description())
	}

	if entry.Text() != current.Text() {
		bulk.AddItem("Text", current.Text(), entry.Text())
		current.SetText(entry.Text())
	}

	if strings.Join(entry.Tags(), "|") != strings.Join(current.Tags(), "|") {
		bulk.AddItem("Tags", strings.Join(current.Tags(), ", "), strings.Join(entry.Tags(), ", "))
		current.SetTags(entry.Tags())
	}

	if bulk.HasUpdates() {
		if err := bm.am.AddEntityChangeLog(bulk, session); err != nil {
			return err
		}

		bm.slugCache.Remove(entry.Slug())

		rows := bm.cql.Query(
			"update blog_entry set title=?, slug=?, description=?, tags=?, date=?, created=?, updated=?, author=?, text=? where uuid=?",
			current.Title(),
			current.Slug(),
			current.Description(),
			current.Tags(),
			current.Date(),
			current.Created(),
			current.Updated(),
			current.Author(),
			current.Text(),
			current.SearchTags(),
			current.Uuid()).Iter()
		err := rows.Close()
		if err != nil {
			return err
		}

		bm.slugCache.Remove(entry.Slug())
		bm.slugCache.Set(entry.Slug(), entry)
	}

	return nil
}

// DeleteEntry removes a blog entry from the database. It does not remove
// entity change history, so theoretically the data is recoverable by a
// programmer if the situation calls for recovery of a blog entry.
func (bm *CqlBlogManager) DeleteEntry(uuid string, session security.Session) error {
	if uuid == "" {
		return errors.New("Cannot delete entry without a uuid")
	}
	if session == nil || !session.IsAuthenticated() {
		return &security.ErrUnauthenticated{session}
	}

	// Must fetch first so we know the slug, so we can clear the slug
	// from the cache
	entry, err := bm.GetEntryCached(uuid, session)
	if err != nil {
		return err
	}
	if entry == nil {
		return errors.New("No entry has this uuid")
	}

	rows := bm.cql.Query("delete from blog_entry where uuid=?", uuid).Iter()
	err = rows.Close()
	if err != nil {
		return err
	}

	bm.entryCache.Remove(uuid)
	bm.slugCache.Remove(entry.Slug())

	return nil
}
