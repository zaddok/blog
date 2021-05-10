package blog

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/bluele/gcache"
	"github.com/gocql/gocql"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/zaddok/log"
	"gitlab.com/montebo/security"
	"golang.org/x/text/language"
)

func NewCqlBlogManager(cql *gocql.Session, am security.AccessManager, log log.Log) (*CqlBlogManager, error) {
	s := &CqlBlogManager{
		cql:        cql,
		am:         am,
		log:        log,
		entryCache: gcache.New(200).LRU().Expiration(time.Second * 3600).Build(),
		slugCache:  gcache.New(200).LRU().Expiration(time.Second * 3600).Build(),
	}
	am.AddCustomRoleType("User", "bk1", "Manage Blog Entries", "Create and update blog entries")

	rows := cql.Query(`
create table if not exists blog_entry (
	site text,
	uuid text,
	slug text,
	title text,
	description text,
	thumbnail text,
	cover text,
	tags set<text>,
	search_tags set<text>,
	"date" timestamp,
	created timestamp,
	updated timestamp,
	author text,
	text text,
	html text,
	deleted boolean,
	primary key ((site), uuid))
`).Iter()
	err := rows.Close()
	if err != nil {
		return nil, err
	}

	rows = cql.Query(`create index if not exists blog_slug on blog_entry (slug)`).Iter()
	err = rows.Close()
	if err != nil {
		return nil, errors.New("blog_slug creation failed. " + err.Error())
	}

	rows = cql.Query(`create index if not exists blog_entry_search on blog_entry (search_tags)`).Iter()
	err = rows.Close()
	if err != nil {
		return nil, err
	}

	activateBlogPlugin(am)

	return s, nil
}

func activateBlogPlugin(am security.AccessManager) {
	security.RegisterSecondaryMenuItem(security.ActionButton{
		Title: "manage-blog",
		Link:  "/blog/manage",
		Roles: []string{"bk1"},
	})

	security.RegisterTranslations(language.English,
		&i18n.Message{ID: "blog", Other: "Blog"},
		&i18n.Message{ID: "manage-blog", Other: "Manage Blog"},
	)

	security.RegisterTranslations(language.TraditionalChinese,
		&i18n.Message{ID: "blog", Other: "部落格"},
		&i18n.Message{ID: "manage-blog", Other: "Manage 部落格"},
	)

	security.RegisterTranslations(language.SimplifiedChinese,
		&i18n.Message{ID: "blog", Other: "博客"},
		&i18n.Message{ID: "manage-blog", Other: "Manage 博客"},
	)

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

	rows := bm.cql.Query("select title, slug, description, tags, date, created, updated, author, text, thumbnail, cover, deleted from blog_entry where site=? and uuid=?",
		session.Site(), uuid).Iter()
	if !rows.Scan(&entry.title, &entry.slug, &entry.description, &entry.tags, &entry.date, &entry.created, &entry.updated, &entry.authorUuid, &entry.text, &entry.thumbnail, &entry.cover, &entry.deleted) {
		return nil, rows.Close()
	}

	// Found result
	err := rows.Close()
	if err != nil {
		return nil, err
	}

	entry.uuid = uuid

	if entry.authorUuid != "" {
		entry.author, err = bm.am.GetPersonCached(entry.authorUuid, session)
		if err != nil {
			return nil, err
		}
	}

	bm.entryCache.Set(entry.Uuid(), entry)
	bm.slugCache.Set(entry.Slug(), entry)

	return &entry, nil
}

func (bm *CqlBlogManager) GetEntries(session security.Session) ([]Entry, error) {

	if session == nil {
		return nil, errors.New("Invalid session object. Contact support.")
	}

	var items []Entry
	var err error

	rows := bm.cql.Query("select uuid, title, slug, description, tags, date, created, updated, author, text, thumbnail, cover, deleted from blog_entry where site=?", session.Site()).Iter()
	entry := &GaeEntry{}
	for rows.Scan(&entry.uuid, &entry.title, &entry.slug, &entry.description, &entry.tags, &entry.date, &entry.created, &entry.updated, &entry.authorUuid, &entry.text, &entry.thumbnail, &entry.cover, &entry.deleted) {
		if entry.authorUuid != "" {
			entry.author, err = bm.am.GetPersonCached(entry.authorUuid, session)
			if err != nil {
				return nil, err
			}
		}
		items = append(items, entry)

		bm.entryCache.Set(entry.Uuid(), entry)
		bm.slugCache.Set(entry.Slug(), entry)
		entry = &GaeEntry{}
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

	return items[:], nil
}

func (bm *CqlBlogManager) GetRecentEntries(limit int, session security.Session) ([]Entry, error) {

	if session == nil {
		return nil, errors.New("Invalid session object. Contact support.")
	}

	var items []Entry
	var err error
	now := time.Now()

	rows := bm.cql.Query("select uuid, title, slug, description, tags, date, created, updated, author, text, thumbnail, cover, deleted from blog_entry where site=?", session.Site()).Iter()
	entry := &GaeEntry{}
	for rows.Scan(&entry.uuid, &entry.title, &entry.slug, &entry.description, &entry.tags, &entry.date, &entry.created, &entry.updated, &entry.authorUuid, &entry.text, &entry.thumbnail, &entry.cover, &entry.deleted) {
		if entry.date.Before(now) {
			if entry.authorUuid != "" {
				entry.author, err = bm.am.GetPersonCached(entry.authorUuid, session)
				if err != nil {
					return nil, err
				}
			}
			items = append(items, entry)

			bm.entryCache.Set(entry.Uuid(), entry)
			bm.slugCache.Set(entry.Slug(), entry)
			entry = &GaeEntry{}
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

func (bm *CqlBlogManager) GetEntriesByTag(tag string, limit int, session security.Session) ([]Entry, error) {

	if session == nil {
		return nil, errors.New("Invalid session object. Contact support.")
	}

	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return nil, nil
	}

	var items []Entry
	var err error
	now := time.Now()

	rows := bm.cql.Query("select uuid, title, slug, description, tags, date, created, updated, author, text, thumbnail, cover, deleted from blog_entry where site=? and search_tags contains ?", session.Site(), "tag:"+tag).Iter()
	entry := &GaeEntry{}
	for rows.Scan(&entry.uuid, &entry.title, &entry.slug, &entry.description, &entry.tags, &entry.date, &entry.created, &entry.updated, &entry.authorUuid, &entry.text, &entry.thumbnail, &entry.cover, &entry.deleted) {
		if entry.date.Before(now) {
			if entry.authorUuid != "" {
				entry.author, err = bm.am.GetPersonCached(entry.authorUuid, session)
				if err != nil {
					return nil, err
				}
			}
			items = append(items, entry)

			bm.entryCache.Set(entry.Uuid(), entry)
			bm.slugCache.Set(entry.Slug(), entry)
			entry = &GaeEntry{}
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

	if session == nil {
		return nil, errors.New("Invalid session object. Contact support.")
	}

	var items []Entry
	var err error
	now := time.Now()

	rows := bm.cql.Query("select uuid, title, slug, description, tags, date, created, updated, author, text, thumbnail, cover, deleted from blog_entry where site=? and author=?", session.Site(), personUuid).Iter()
	entry := &GaeEntry{}
	for rows.Scan(&entry.uuid, &entry.title, &entry.slug, &entry.description, &entry.tags, &entry.date, &entry.created, &entry.updated, &entry.authorUuid, &entry.text, &entry.thumbnail, &entry.cover, &entry.deleted) {
		if entry.date.After(now) {
			if entry.authorUuid != "" {
				entry.author, err = bm.am.GetPersonCached(entry.authorUuid, session)
				if err != nil {
					return nil, err
				}
			}
			items = append(items, entry)
			entry = &GaeEntry{}
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

	if session == nil {
		return nil, errors.New("Invalid session object. Contact support.")
	}

	var items []Entry
	var err error

	fields := strings.Fields(strings.ToLower(query))
	sort.Slice(fields, func(i, j int) bool {
		return len(fields[j]) < len(fields[i])
	})

	if len(fields) == 0 {
		return nil, nil
	}

	rows := bm.cql.Query("select uuid, title, slug, description, tags, date, created, updated, author, text, thumbnail, cover, deleted from blog_entry where site=? and search_tags=?", session.Site(), fields).Iter()
	entry := &GaeEntry{}
	for rows.Scan(&entry.uuid, &entry.title, &entry.slug, &entry.description, &entry.tags, &entry.date, &entry.created, &entry.updated, &entry.authorUuid, &entry.text, &entry.thumbnail, &entry.cover, &entry.deleted) {
		if entry.authorUuid != "" {
			entry.author, err = bm.am.GetPersonCached(entry.authorUuid, session)
			if err != nil {
				return nil, err
			}
		}
		items = append(items, entry)
		entry = &GaeEntry{}
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

	if session == nil {
		return nil, errors.New("Invalid session object. Contact support.")
	}

	var items []Entry
	var err error
	now := time.Now()

	rows := bm.cql.Query("select uuid, title, slug, description, tags, date, created, updated, author, text, html, thumbnail, cover, deleted from blog_entry where site=?", session.Site()).Iter()
	entry := &GaeEntry{}
	for rows.Scan(&entry.uuid, &entry.title, &entry.slug, &entry.description, &entry.tags, &entry.date, &entry.created, &entry.updated, &entry.authorUuid, &entry.text, &entry.html, &entry.thumbnail, &entry.cover, &entry.deleted) {
		if entry.date.After(now) {
			if entry.authorUuid != "" {
				entry.author, err = bm.am.GetPersonCached(entry.authorUuid, session)
				if err != nil {
					return nil, err
				}
			}
			items = append(items, entry)
			entry = &GaeEntry{}
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

	if session == nil {
		return nil, errors.New("Invalid session object. Contact support.")
	}

	var entry GaeEntry

	rows := bm.cql.Query("select uuid, title, slug, description, tags, date, created, updated, author, text, html, thumbnail, cover, deleted from blog_entry where site=? and slug=?",
		session.Site(), slug).Iter()
	if !rows.Scan(&entry.uuid, &entry.title, &entry.slug, &entry.description, &entry.tags, &entry.date, &entry.created, &entry.updated, &entry.authorUuid, &entry.text, &entry.html, &entry.thumbnail, &entry.cover, &entry.deleted) {
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

	bm.entryCache.Set(entry.Uuid(), entry)
	bm.slugCache.Set(entry.Slug(), entry)

	return &entry, nil
}

func (bm *CqlBlogManager) GetEntryCached(uuid string, session security.Session) (Entry, error) {

	if session == nil {
		return nil, errors.New("Invalid session object. Contact support.")
	}

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

	if session == nil {
		return nil, errors.New("Invalid session object. Contact support.")
	}

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
	if entry == nil {
		return nil, nil
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

	if entry.Text() != "" {
		bulk.AddItem("Thumbnail", "", entry.Thumbnail())
	}

	if entry.Text() != "" {
		bulk.AddItem("Cover", "", entry.Cover())
	}

	if len(entry.Tags()) > 0 {
		bulk.AddItem("Tags", "", strings.Join(entry.Tags(), ", "))
	}

	if entry.Author() != nil {
		bulk.AddItem("Author", "", entry.Author().Uuid())
	}

	if entry.Deleted() {
		bulk.AddBoolItem("Deleted", false, true)
	}

	now := time.Now()
	entry.setCreated(now)
	entry.setUpdated(now)

	// TODO: Technically should be in a transaction
	if err := bm.am.AddEntityChangeLog(bulk, session); err != nil {
		return err
	}

	rows := bm.cql.Query(
		"update blog_entry set title=?, slug=?, description=?, tags=?, date=?, created=?, updated=?, author=?, text=?, html=?, thumbnail=?, cover=?, search_tags=?, deleted=? where site=? and uuid=?",
		entry.Title(),
		entry.Slug(),
		entry.Description(),
		entry.Tags(),
		entry.Date(),
		entry.Created(),
		entry.Updated(),
		entry.AuthorUUID(),
		entry.Text(),
		entry.Html(),
		entry.Thumbnail(),
		entry.Cover(),
		entry.SearchTags(),
		entry.Deleted(),
		session.Site(),
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
	rows := bm.cql.Query("select title, slug, description, tags, date, created, updated, author, deleted, text, html, thumbnail, cover from blog_entry where site=? and uuid=?",
		session.Site(), entry.Uuid()).Iter()
	if !rows.Scan(
		&current.title,
		&current.slug,
		&current.description,
		&current.tags,
		&current.date,
		&current.created,
		&current.updated,
		&current.authorUuid,
		&current.deleted,
		&current.text,
		&current.html,
		&current.thumbnail,
		&current.cover) {
		err := rows.Close()
		if err == nil {
			return errors.New("No entry has uuid " + entry.Uuid() + " on site " + session.Site())
		}
		return err
	}
	current.uuid = entry.Uuid()
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

	if entry.Thumbnail() != current.Thumbnail() {
		bulk.AddItem("Thumbnail", current.Thumbnail(), entry.Thumbnail())
		current.SetThumbnail(entry.Thumbnail())
	}

	if entry.Cover() != current.Cover() {
		bulk.AddItem("Cover", current.Cover(), entry.Cover())
		current.SetCover(entry.Cover())
	}

	if strings.Join(entry.Tags(), "|") != strings.Join(current.Tags(), "|") {
		bulk.AddItem("Tags", strings.Join(current.Tags(), ", "), strings.Join(entry.Tags(), ", "))
		current.SetTags(entry.Tags())
	}

	if bulk.HasUpdates() {
		if err := bm.am.AddEntityChangeLog(bulk, session); err != nil {
			return err
		}

		now := time.Now()
		current.updated = &now

		bm.slugCache.Remove(entry.Slug())
		rows := bm.cql.Query(
			"update blog_entry set title=?, slug=?, description=?, tags=?, date=?, updated=?, author=?, text=?, html=?, deleted=?, search_tags=?, thumbnail=?, cover=? where site=? and uuid=?",
			current.Title(),
			current.Slug(),
			current.Description(),
			current.Tags(),
			current.Date(),
			current.Updated(),
			current.AuthorUUID(),
			current.Text(),
			current.Html(),
			current.Deleted(),
			current.SearchTags(),
			current.Thumbnail(),
			current.Cover(),
			session.Site(),
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
