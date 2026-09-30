package console

// entryHistory is a bounded arrival-order ring. Hidden polling and visible
// activity get separate histories so one cannot evict the other. Strings are
// shared between them; neither history grows with the lifetime of the app.
type entryHistory struct {
	items  []entry
	oldest int
}

func (h *entryHistory) add(e entry) {
	if h.items == nil {
		h.items = make([]entry, 0, maxEntries)
	}
	if len(h.items) < maxEntries {
		h.items = append(h.items, e)
		return
	}
	h.items[h.oldest] = e
	h.oldest = (h.oldest + 1) % maxEntries
}

func (h *entryHistory) snapshot() []entry {
	entries := make([]entry, 0, len(h.items))
	entries = append(entries, h.items[h.oldest:]...)
	return append(entries, h.items[:h.oldest]...)
}
