package worm

// ExportRecord is the sink write result (WORM output metadata).
// This is NOT a DB SoT; it is runtime output only.
type ExportRecord struct {
	ObjectKey string
	Bytes     int64
	Sink      string
}