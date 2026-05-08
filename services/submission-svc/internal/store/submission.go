package store

import "time"

type Language string

const (
	LangCPP  Language = "cpp"
	LangRust Language = "rust"
	LangGo   Language = "go"
)

func (l Language) Valid() bool {
	switch l {
	case LangCPP, LangRust, LangGo:
		return true
	}
	return false
}

type Status string

const (
	StatusUploaded    Status = "uploaded"
	StatusBuilding    Status = "building"
	StatusBuildFailed Status = "build_failed"
	StatusReady       Status = "ready"
	StatusRunning     Status = "running"
	StatusCompleted   Status = "completed"
	StatusCrashed     Status = "crashed"
)

type Submission struct {
	ID           string    `json:"id"`
	ContestantID string    `json:"contestant_id"`
	Language     Language  `json:"language"`
	Status       Status    `json:"status"`
	SourceURI    string    `json:"source_uri"`
	ImageURI     string    `json:"image_uri,omitempty"`
	Entrypoint   string    `json:"entrypoint"`
	BuildError   string    `json:"build_error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Repository interface {
	Create(s Submission) error
	Get(id string) (Submission, bool)
	UpdateStatus(id string, status Status, imageURI, buildError string) error
	List(contestantID string, limit int) []Submission
}
