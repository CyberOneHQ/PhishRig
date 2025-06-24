package store

// Repository defines the data access interface for the application
type Repository interface {
	UpsertEngagement(e Engagement) error
	GetEngagement(id string) (*Engagement, error)
	GetActiveEngagement() (*Engagement, error)
	InsertCredential(c CapturedCredential) error
	GetCredentials(engagementID string) ([]CapturedCredential, error)
	GetAllCredentials() ([]CapturedCredential, error)
	CredentialCount(engagementID string) (int, error)
	Close() error
}

// Ensure DB implements Repository at compile time
var _ Repository = (*DB)(nil)
