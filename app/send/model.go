package send

import "time"

const (
	kindVideo    = "video"
	kindPhoto    = "photo"
	kindDocument = "document"
)

const (
	Pending        = "pending"
	Uploading      = "uploading"
	Sending        = "sending"
	Sent           = "sent"
	Verified       = "verified"
	Failed         = "failed"
	Uncertain      = "uncertain"
	Missing        = "missing"
	Review         = "review"
	NeedsTranscode = "needs_transcode"
)

type Destination struct {
	PeerID  int64  `json:"peer_id"`
	Topic   int    `json:"topic"`
	Name    string `json:"name"`
	Link    string `json:"link"`
	FloorID int    `json:"floor_id"`
}

type File struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Hash string `json:"sha256"`
	Size int64  `json:"size"`
	Kind string `json:"kind"`
}

type MediaRef struct {
	Kind     string  `json:"kind"`
	ID       int64   `json:"id"`
	Size     int64   `json:"size"`
	Duration float64 `json:"duration,omitempty"`
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
}

type Entry struct {
	File       File      `json:"file"`
	Status     string    `json:"status"`
	RandomID   int64     `json:"random_id"`
	MessageID  int       `json:"message_id,omitempty"`
	Media      MediaRef  `json:"media"`
	Caption    string    `json:"caption,omitempty"`
	Error      string    `json:"error,omitempty"`
	Attempts   int       `json:"attempts"`
	Transcoded bool      `json:"transcoded,omitempty"`
	Updated    time.Time `json:"updated"`
}

type Task struct {
	Version     int               `json:"version"`
	ID          string            `json:"id"`
	Root        string            `json:"root"`
	Namespace   string            `json:"namespace"`
	AccountID   int64             `json:"account_id"`
	Destination Destination       `json:"destination"`
	Created     time.Time         `json:"created"`
	Entries     map[string]*Entry `json:"entries"`
}

type Scan struct {
	Root       string
	Count      int
	Duplicates int
	Ignored    int
	Files      []File
	Errors     []string
}

type Summary struct {
	TaskID      string   `json:"task_id"`
	Destination string   `json:"destination"`
	Scanned     int      `json:"scanned"`
	Duplicates  int      `json:"duplicates"`
	Ignored     int      `json:"ignored"`
	Expected    int      `json:"expected"`
	Verified    int      `json:"verified"`
	Videos      int      `json:"videos"`
	Photos      int      `json:"photos"`
	Documents   int      `json:"documents"`
	Transcoded  int      `json:"transcoded"`
	Unresolved  int      `json:"unresolved"`
	Issues      []string `json:"issues,omitempty"`
}
