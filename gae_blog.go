package blog

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/bluele/gcache"
	"gitlab.com/montebo/security"
	"google.golang.org/api/iterator"
)

func NewGaeBlogManager(client *datastore.Client, ctx context.Context, am security.AccessManager) *GaeBlogManager {
	s := &GaeBlogManager{
		client:     client,
		ctx:        ctx,
		am:         am,
		entryCache: gcache.New(200).LRU().Expiration(time.Second * 3600).Build(),
		slugCache:  gcache.New(200).LRU().Expiration(time.Second * 3600).Build(),
	}
	return s
}

type GaeBlogManager struct {
	client     *datastore.Client
	ctx        context.Context
	am         security.AccessManager
	entryCache gcache.Cache
	slugCache  gcache.Cache
}

func (em *GaeBlogManager) NewEntry() Entry {
	return &GaeEntry{}
}

func (em *GaeBlogManager) AccessManager() security.AccessManager {
	return em.am
}

func (em *GaeBlogManager) GetEntry(uuid string, session security.Session) (Entry, error) {
	item := new(GaeEntry)
	k := datastore.NameKey("Entry", uuid, nil)
	k.Namespace = session.Site()
	err := em.client.Get(em.ctx, k, item)
	if err == datastore.ErrNoSuchEntity {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if item.authorUuid != "" {
		item.author, err = em.am.GetPersonCached(item.authorUuid, session)
		if err != nil {
			return nil, err
		}
	}
	return item, nil
}

func (em *GaeBlogManager) GetRecentEntries(limit int, session security.Session) ([]Entry, error) {
	var items []Entry
	var err error

	q := datastore.NewQuery("Entry").Namespace(session.Site()).Filter("Date <", time.Now()).Limit(limit)
	it := em.client.Run(em.ctx, q)
	for {
		e := new(GaeEntry)
		if _, err := it.Next(e); err == iterator.Done {
			break
		} else if err != nil {
			return nil, err
		}
		if e.authorUuid != "" {
			e.author, err = em.am.GetPersonCached(e.authorUuid, session)
			if err != nil {
				return nil, err
			}
		}
		items = append(items, e)
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

func (em *GaeBlogManager) GetFutureEntries(session security.Session) ([]Entry, error) {
	var items []Entry
	var err error

	q := datastore.NewQuery("Entry").Namespace(session.Site()).Filter("Date >", time.Now())
	it := em.client.Run(em.ctx, q)
	for {
		e := new(GaeEntry)
		if _, err := it.Next(e); err == iterator.Done {
			break
		} else if err != nil {
			return nil, err
		}
		if e.authorUuid != "" {
			e.author, err = em.am.GetPersonCached(e.authorUuid, session)
			if err != nil {
				return nil, err
			}
		}
		items = append(items, e)
	}

	return items, nil
}

func (em *GaeBlogManager) GetEntryBySlug(slug string, session security.Session) (Entry, error) {
	var items []GaeEntry
	var err error

	q := datastore.NewQuery("Entry").Namespace(session.Site()).Filter("Slug =", slug).Limit(1)
	_, err = em.client.GetAll(em.ctx, q, &items)
	if err != nil {
		return nil, err
	}

	if len(items) > 0 {
		if items[0].authorUuid != "" {
			items[0].author, err = em.am.GetPersonCached(items[0].authorUuid, session)
			if err != nil {
				return nil, err
			}
		}
		return &items[0], nil
	}
	return nil, nil
}

func (em *GaeBlogManager) GetEntryCached(uuid string, session security.Session) (Entry, error) {

	if uuid == "" {
		return nil, nil
	}

	v, _ := em.entryCache.Get(uuid)
	if v != nil {
		entry := v.(Entry)
		return entry, nil
	}

	entry, err := em.GetEntry(uuid, session)
	if err != nil {
		return nil, err
	}
	em.entryCache.Set(entry.Uuid(), entry)
	em.slugCache.Set(entry.Slug(), entry)

	return entry, nil
}

func (em *GaeBlogManager) GetEntryBySlugCached(slug string, session security.Session) (Entry, error) {

	if slug == "" {
		return nil, nil
	}

	v, _ := em.slugCache.Get(slug)
	if v != nil {
		entry := v.(Entry)
		return entry, nil
	}

	entry, err := em.GetEntryBySlug(slug, session)
	if err != nil {
		return nil, err
	}
	em.entryCache.Set(entry.Uuid(), entry)
	em.slugCache.Set(entry.Slug(), entry)

	return entry, nil
}

func (em *GaeBlogManager) AddEntry(entry Entry, session security.Session) error {
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

	k := datastore.NameKey("Entry", entry.Uuid(), nil)
	k.Namespace = session.Site()

	// TODO: Technically should be in a transaction
	if err := em.am.AddEntityChangeLog(bulk, session); err != nil {
		return err
	}
	if _, err := em.client.Put(em.ctx, k, entry.(*GaeEntry)); err != nil {
		return err
	}

	em.entryCache.Set(entry.Uuid(), entry)
	em.slugCache.Set(entry.Slug(), entry)

	return nil
}

func (em *GaeBlogManager) UpdateEntry(entry Entry, session security.Session) error {
	if session == nil || !session.IsAuthenticated() {
		return &security.ErrUnauthenticated{session}
	}

	if entry.Title() == "" {
		return errors.New("Entry must have a title")
	}
	if entry.Text() == "" {
		return errors.New("Entry must contain text")
	}

	k := datastore.NameKey("Entry", entry.Uuid(), nil)
	k.Namespace = session.Site()

	current := new(GaeEntry)
	err := em.client.Get(em.ctx, k, current)
	if err == datastore.ErrNoSuchEntity {
		return errors.New("No entry has this uuid")
	} else if err != nil {
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
		if err := em.am.AddEntityChangeLog(bulk, session); err != nil {
			return err
		}

		em.entryCache.Remove(entry.Uuid())
		em.slugCache.Remove(entry.Slug())
		if _, err := em.client.Put(em.ctx, k, current); err != nil {
			return err
		}
		em.entryCache.Set(entry.Uuid(), entry)
		em.slugCache.Set(entry.Slug(), entry)
	}

	return nil
}

func (em *GaeBlogManager) DeleteEntry(uuid string, session security.Session) error {
	if uuid == "" {
		return errors.New("Cannot delete entry without a uuid")
	}
	if session == nil || !session.IsAuthenticated() {
		return &security.ErrUnauthenticated{session}
	}
	k := datastore.NameKey("Entry", uuid, nil)
	k.Namespace = session.Site()
	var current GaeEntry
	err := em.client.Get(em.ctx, k, &current)
	if err == datastore.ErrNoSuchEntity {
		return errors.New("No entry has this uuid")
	} else if err != nil {
		return err
	}

	em.entryCache.Remove(current.Uuid())
	em.slugCache.Remove(current.Slug())

	return nil
}

func (em *GaeBlogManager) GetEntriesByAuthor(personUuid string, session security.Session) ([]Entry, error) {
	items := make([]Entry, 0)

	q := datastore.NewQuery("Entry").Namespace(session.Site()).Filter("Author =", personUuid).Limit(5000)
	_, err := em.client.GetAll(em.ctx, q, &items)
	if err != nil {
		return nil, err
	}

	return items, nil
}

func (em *GaeBlogManager) SearchEntries(query string, session security.Session) ([]Entry, error) {
	var err error
	results := make([]Entry, 0)

	fields := strings.Fields(strings.ToLower(query))
	sort.Slice(fields, func(i, j int) bool {
		return len(fields[j]) < len(fields[i])
	})

	if len(fields) == 0 {
		return results, nil
	} else if len(fields) == 1 {
		q := datastore.NewQuery("Entry").Namespace(session.Site()).Filter("SearchTags =", fields[0]).Limit(50)
		it := em.client.Run(em.ctx, q)
		for {
			e := new(GaeEntry)
			if _, err := it.Next(e); err == iterator.Done {
				break
			} else if err != nil {
				return nil, err
			}
			if e.authorUuid != "" {
				e.author, err = em.am.GetPersonCached(e.authorUuid, session)
				if err != nil {
					return nil, err
				}
			}
			results = append(results, e)
		}
	} else if len(fields) > 1 {
		q := datastore.NewQuery("Entry").Namespace(session.Site()).Filter("SearchTags =", fields[0]).Filter("SearchTags =", fields[1]).Limit(50)
		it := em.client.Run(em.ctx, q)
		for {
			e := new(GaeEntry)
			if _, err := it.Next(e); err == iterator.Done {
				break
			} else if err != nil {
				return nil, err
			}
			if e.authorUuid != "" {
				e.author, err = em.am.GetPersonCached(e.authorUuid, session)
				if err != nil {
					return nil, err
				}
			}
			results = append(results, e)
		}
	}

	return results, nil
}
