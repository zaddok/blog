package blog

import (
	"time"

	"gitlab.com/montebo/security"
)

type Entry interface {
	Uuid() string
	Title() string
	Slug() string
	Description() string
	Tags() []string
	Date() *time.Time
	Author() security.Person
	Text() string
	Html() string
	Created() *time.Time
	Updated() *time.Time

	SetTitle(title string)
	SetDescription(description string)
	SetTags(tags []string)
	SetDate(date time.Time)
	SetAuthor(author security.Person)
	SetText(text string)
}

type BlogManager interface {
	GetEntry(uuid string, session security.Session) (Entry, error)
	GetEntryCached(uuid string, session security.Session) (Entry, error)
	GetEntryBySlug(slug string, session security.Session) (Entry, error)
	GetEntryBySlugCached(slug string, session security.Session) (Entry, error)
	GetRecentEntries(limit int, session security.Session) ([]Entry, error)
	GetFutureEntries(session security.Session) ([]Entry, error)
	GetEntriesByAuthor(personUuid string, session security.Session) ([]Entry, error)
	SearchEntries(query string, session security.Session) ([]Entry, error)

	AddEntry(entry Entry, session security.Session) error
	UpdateEntry(event Entry, session security.Session) error
	DeleteEntry(uuid string, session security.Session) error

	NewEntry() Entry
}
