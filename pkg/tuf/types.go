package tuf

// TUF metadata structures for The Update Framework

type Key struct {
	KeyType string `json:"keytype"`
	Scheme  string `json:"scheme"`
	KeyVal  struct {
		Public string `json:"public"`
	} `json:"keyval"`
}

type Role struct {
	KeyIDs    []string `json:"keyids"`
	Threshold int      `json:"threshold"`
}

type Root struct {
	Type    string            `json:"_type"`
	Version int               `json:"version"`
	Expires string            `json:"expires"`
	Keys    map[string]Key    `json:"keys"`
	Roles   map[string]Role   `json:"roles"`
}

type TargetInfo struct {
	Length int               `json:"length"`
	Hashes map[string]string `json:"hashes"`
}

type Targets struct {
	Type    string                  `json:"_type"`
	Version int                     `json:"version"`
	Expires string                  `json:"expires"`
	Targets map[string]TargetInfo   `json:"targets"`
}

type Snapshot struct {
	Type    string `json:"_type"`
	Version int    `json:"version"`
	Expires string `json:"expires"`
	Meta    map[string]struct {
		Version int `json:"version"`
	} `json:"meta"`
}

type Timestamp struct {
	Type    string `json:"_type"`
	Version int    `json:"version"`
	Expires string `json:"expires"`
	Meta    map[string]struct {
		Version int `json:"version"`
	} `json:"meta"`
}