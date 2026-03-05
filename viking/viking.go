package viking

type Entry struct {
	Key     string
	Content string
}

type Store interface {
	Save(entry Entry) error
	Load(key string) (Entry, error)
	Delete(key string) error
	List() ([]Entry, error)
}
