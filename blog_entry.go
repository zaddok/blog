package blog

import (
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/google/uuid"
	"gitlab.com/montebo/security"
)

type GaeEntry struct {
	uuid        string
	title       string
	slug        string
	description string
	tags        []string
	date        *time.Time
	authorUuid  string
	text        string
	created     *time.Time
	updated     *time.Time

	html   string
	author security.Person
}

func (e *GaeEntry) Uuid() string {
	if e.uuid == "" {
		// TODO: Can this err out, and if so do what?
		u, _ := uuid.NewUUID()
		e.uuid = u.String()
	}
	return e.uuid
}

func (e *GaeEntry) Title() string {
	return e.title
}

func (e *GaeEntry) SetTitle(title string) {
	e.title = title
	e.slug = Slugify(title)
}

func (e *GaeEntry) Slug() string {
	if e.slug == "" && e.title != "" {
		e.slug = Slugify(e.title)
	}
	return e.slug
}

func (e *GaeEntry) Description() string {
	return e.description
}

func (e *GaeEntry) SetDescription(description string) {
	e.description = description
}

func (e *GaeEntry) Tags() []string {
	return e.tags
}

func (e *GaeEntry) SetTags(tags []string) {
	e.tags = tags
}

func (e *GaeEntry) Date() *time.Time {
	return e.date
}

func (e *GaeEntry) SetDate(date time.Time) {
	e.date = &date
}

func (e *GaeEntry) Author() security.Person {
	return e.author
}

func (e *GaeEntry) SetAuthor(author security.Person) {
	e.author = author
	if e.author != nil {
		e.authorUuid = author.Uuid()
	}
}

func (e *GaeEntry) Text() string {
	return e.text
}

func (e *GaeEntry) SetText(text string) {
	e.text = text
}

func (e *GaeEntry) Html() string {
	return e.text
}

func (e *GaeEntry) Created() *time.Time {
	return e.updated
}

func (e *GaeEntry) Updated() *time.Time {
	return e.updated
}

func (e *GaeEntry) LoadKey(k *datastore.Key) error {
	if k != nil {
		e.uuid = k.Name
	}
	return nil
}

func (e *GaeEntry) Load(ps []datastore.Property) error {
	for _, i := range ps {
		switch i.Name {
		case "Title":
			e.title = i.Value.(string)
			break
		case "Slug":
			e.slug = i.Value.(string)
			break
		case "Description":
			e.description = i.Value.(string)
			break
		case "Tags":
			e.tags = strings.Split(i.Value.(string), "|")
			break
		case "Date":
			if i.Value != nil {
				t := i.Value.(time.Time)
				e.date = &t
			}
			break
		case "Created":
			if i.Value != nil {
				t := i.Value.(time.Time)
				e.created = &t
			}
			break
		case "Updated":
			if i.Value != nil {
				t := i.Value.(time.Time)
				e.updated = &t
			}
			break
		case "Author":
			e.authorUuid = i.Value.(string)
			break
		case "Text":
			e.text = i.Value.(string)
			break
		}
	}
	return nil
}

func (e *GaeEntry) Save() ([]datastore.Property, error) {
	props := []datastore.Property{
		{
			Name:  "Title",
			Value: e.title,
		},
		{
			Name:  "Slug",
			Value: e.slug,
		},
		{
			Name:    "Description",
			Value:   e.description,
			NoIndex: true,
		},
		{
			Name:    "Text",
			Value:   e.text,
			NoIndex: true,
		},
		{
			Name:  "Author",
			Value: e.authorUuid,
		},
	}

	if len(e.tags) > 0 {
		props = append(props, datastore.Property{Name: "Tags", Value: strings.Join(e.tags, "|")})
	}

	if e.date != nil {
		props = append(props, datastore.Property{Name: "Date", Value: e.date})
	}

	now := time.Now()

	if e.created != nil {
		e.created = &now
		props = append(props, datastore.Property{Name: "Created", Value: e.created})
	}

	if e.updated != nil {
		e.updated = &now
		props = append(props, datastore.Property{Name: "Updated", Value: e.updated})
	}

	props = append(props, datastore.Property{Name: "SearchTags", Value: e.SearchTagsI()})

	return props, nil
}

func (e *GaeEntry) SearchTagsI() []interface{} {
	searchTags := e.SearchTags()
	var tags []interface{}
	for _, tag := range searchTags {
		tags = append(tags, tag)
	}
	return tags[:]
}

func (e *GaeEntry) SearchTags() []string {
	var tags []string

	for _, r := range strings.Fields(strings.ToLower(e.Title())) {
		if r != "" {
			tags = append(tags, r)
		}
	}

	for _, tag := range e.Tags() {
		tag = strings.ToLower(strings.ReplaceAll(tag, " ", "-"))
		if tag != "" {
			tags = append(tags, tag)
			tags = append(tags, "tag:"+tag)
		}
	}

	if e.Date() != nil {
		tags = append(tags, fmt.Sprintf("%d", e.Date().Year()))
	}

	if e.Author() != nil {
		if e.Author().FirstName() != "" {
			tags = append(tags, strings.ToLower(e.Author().FirstName()))
		}
		if e.Author().LastName() != "" {
			tags = append(tags, strings.ToLower(e.Author().LastName()))
		}
	}

	return tags[:]
}
