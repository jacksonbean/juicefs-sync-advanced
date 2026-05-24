package scan

import (
	"time"
)

type Config struct {
	ScanID    string
	DBType    string
	DBDSN     string
	Prefix    string
	StartTime time.Time
	EndTime   time.Time
	Threads   int
	Limit     int64
	WithHead  bool
	Export    string
}
