package application

type ImportRowPreview struct {
	Row      int      `json:"row"`
	Server   string   `json:"server"`
	Instance string   `json:"instance"`
	Database string   `json:"database"`
	Errors   []string `json:"errors"`
	Values   []string `json:"values"`
}
type ImportPreview struct {
	WithPolicies   bool                    `json:"withPolicies"`
	Policies       []ImportPolicyCandidate `json:"policies"`
	OriginalTotal  int                     `json:"originalTotal"`
	Excluded       int                     `json:"excluded"`
	ExcludedRows   []ImportRowPreview      `json:"excludedRows"`
	Token          string                  `json:"token"`
	Sheet          string                  `json:"sheet"`
	Columns        int                     `json:"columns"`
	Total          int                     `json:"total"`
	Valid          int                     `json:"valid"`
	Ignored        int                     `json:"ignored"`
	SecretsIgnored int                     `json:"secretsIgnored"`
	BackupRows     int                     `json:"backupRows"`
	Rows           []ImportRowPreview      `json:"rows"`
}
type ImportResult struct {
	PoliciesCreated  int `json:"policiesCreated"`
	PoliciesUpdated  int `json:"policiesUpdated"`
	Excluded         int `json:"excluded"`
	ServersCreated   int `json:"serversCreated"`
	InstancesCreated int `json:"instancesCreated"`
	DatabasesCreated int `json:"databasesCreated"`
	ObjectsUpdated   int `json:"objectsUpdated"`
	Skipped          int `json:"skipped"`
}
type ImportRowEdit struct {
	Row    int      `json:"row"`
	Values []string `json:"values"`
}
