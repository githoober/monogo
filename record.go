package monogo

import (
	"time"
)

type Record struct {
	Message   string                 `json:"message"`
	Level     Level                  `json:"level"`
	Channel   string                 `json:"channel"`
	Time      time.Time              `json:"datetime"`
	Context   map[string]interface{} `json:"context"`
	Extra     map[string]interface{} `json:"extra"`
	Formatted string                 `json:"formatted,omitempty"`
}

func (r Record) Clone() Record {
	cloned := r

	if r.Context != nil {
		cloned.Context = make(map[string]interface{}, len(r.Context))
		for k, v := range r.Context {
			cloned.Context[k] = v
		}
	}

	if r.Extra != nil {
		cloned.Extra = make(map[string]interface{}, len(r.Extra))
		for k, v := range r.Extra {
			cloned.Extra[k] = v
		}
	}

	return cloned
}
