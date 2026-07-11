package catalog

import (
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

// Catalog is an immutable snapshot. Constructor inputs and read results are
// defensively copied, so all methods are safe for concurrent readers.
type Catalog struct {
	schemaVersion string
	entries       map[upstream.RouteID]Entry
	ids           []upstream.RouteID
}

func New(document Document, now time.Time) (*Catalog, error) {
	if err := Validate(document, now); err != nil {
		return nil, err
	}
	clone := document.Clone()
	catalog := &Catalog{
		schemaVersion: clone.SchemaVersion,
		entries:       make(map[upstream.RouteID]Entry, len(clone.Entries)),
		ids:           make([]upstream.RouteID, 0, len(clone.Entries)),
	}
	for _, entry := range clone.Entries {
		catalog.entries[entry.ID] = entry
		catalog.ids = append(catalog.ids, entry.ID)
	}
	sort.Slice(catalog.ids, func(i, j int) bool { return catalog.ids[i] < catalog.ids[j] })
	return catalog, nil
}

func (c *Catalog) SchemaVersion() string {
	if c == nil {
		return ""
	}
	return c.schemaVersion
}

func (c *Catalog) Len() int {
	if c == nil {
		return 0
	}
	return len(c.ids)
}

func (c *Catalog) Get(id upstream.RouteID) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	entry, ok := c.entries[id]
	if !ok {
		return Entry{}, false
	}
	return entry.Clone(), true
}

func (c *Catalog) Entries() []Entry {
	if c == nil {
		return nil
	}
	entries := make([]Entry, 0, len(c.ids))
	for _, id := range c.ids {
		entries = append(entries, c.entries[id].Clone())
	}
	return entries
}

func (c *Catalog) Document() Document {
	if c == nil {
		return Document{}
	}
	return Document{SchemaVersion: c.schemaVersion, Entries: c.Entries()}
}
