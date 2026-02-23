package run

type EvidenceAsset struct {
	RunID       string
	TraceID     string
	Kind        string
	ContentType string
	ByteSize    int
	SHA256      string
	FinalURL    string
	StoredPath  string
}