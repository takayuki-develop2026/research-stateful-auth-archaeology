package run

type EvidenceAsset struct {
	RunID   string // uuid string
	TraceID string // uuid string

	Kind        string
	ContentType *string // nullable in DB

	ByteSize   int
	SHA256     string
	FinalURL   string
	StoredPath string
}
